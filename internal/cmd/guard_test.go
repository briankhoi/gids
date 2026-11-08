package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/testenv"
)

func TestMismatchFromLocalOverrideIsFixedThenRerun(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	repo := w.Repo("work/app", true)
	w.Git(repo, "config", "user.email", "oops@personal.com") // e.g. left by gids v1
	out, code := w.commit(repo, "y\n")
	want(t, code == 1, "guard should stop the commit to re-run: %d\n%s", code, out)
	want(t, strings.Contains(out, "mapped to Work") && strings.Contains(out, ".git/config"), "warning: %s", out)
	if v, err := w.TryGit(repo, "config", "--local", "user.email"); err == nil {
		t.Fatalf("local override not removed: %q", v)
	}
	_, code = w.commit(repo, "")
	want(t, code == 0 && w.Author(repo) == workID, "re-run: %d %q", code, w.Author(repo))
	w.check()
}

func TestMismatchDeclinedCommitsAsIs(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	repo := w.Repo("work/app", true)
	w.Git(repo, "config", "user.email", "oops@personal.com")
	_, code := w.commit(repo, "n\n")
	want(t, code == 0, "declining should commit: %d", code)
	want(t, strings.Contains(w.Author(repo), "oops@personal.com"), "author %q", w.Author(repo))
}

func TestMismatchFromAuthorFlagOnlyWarns(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	repo := w.Repo("work/app", true)
	out, code := w.commit(repo, "", "-m", "x", "--author", "Someone <else@x>")
	want(t, code == 0 && strings.Contains(out, "--author"), "%d %s", code, out)
	want(t, w.Author(repo) == "Someone <else@x>", "--author must be respected: %q", w.Author(repo))
}

func TestUnmappedConfirmAndSaveRule(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "profile", "add", "--name", "Global", "--git-name", "Global Me", "--email", "global@x.com")
	repo := w.Repo("projects/foo", true)
	out, code := w.commit(repo, "y\ny\n")
	want(t, code == 0, "%d %s", code, out)
	want(t, w.cfg().Rules["~/projects/foo/"] == "Global", "rules %v", w.cfg().Rules)
	out, code = w.commit(repo, "")
	want(t, code == 0 && !strings.Contains(out, "gids"), "now mapped, should be silent: %q", out)
	w.check()
}

func TestUnmappedConfirmCreatesProfileFromCurrentIdentity(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	_, code := w.commit(repo, "y\nGlobal\ny\n")
	want(t, code == 0 && w.Author(repo) == globalID, "%d %q", code, w.Author(repo))
	p := w.cfg().Profile("Global")
	want(t, p != nil && p.GitEmail == "global@x.com", "profile %+v", p)
	want(t, w.cfg().Rules["~/projects/foo/"] == "Global", "rule missing")
	w.check()
}

func TestUnmappedConfirmSkipEverything(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	before := w.Read(filepath.Join(w.GidsDir(), "config.yaml"))
	_, code := w.commit(repo, "y\n\n") // yes, then Enter = don't save a profile
	want(t, code == 0 && w.Author(repo) == globalID, "%d", code)
	want(t, w.Read(filepath.Join(w.GidsDir(), "config.yaml")) == before, "config changed")
	_, code = w.commit(repo, "y\n\n") // still unmapped: asks every time
	want(t, code == 0, "%d", code)
}

func TestUnmappedWrongIdentityPickProfileThenRerun(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/bot", true)
	head := w.Git(repo, "rev-parse", "HEAD")
	out, code := w.commit(repo, "n\n2\n") // 2 = Personal
	want(t, code == 1 && strings.Contains(out, "Re-run"), "should stop for re-run: %d %s", code, out)
	want(t, w.Git(repo, "rev-parse", "HEAD") == head, "a commit was made with the wrong identity")
	want(t, w.cfg().Rules["~/projects/bot/"] == "Personal", "rule %v", w.cfg().Rules)
	_, code = w.commit(repo, "")
	want(t, code == 0 && w.Author(repo) == meID, "re-run author %q", w.Author(repo))
	w.check()
}

func TestUnmappedWrongIdentityNoProfilesWizard(t *testing.T) {
	w := newWorld(t, false)
	repo := w.Repo("projects/bot", true)
	// n → wizard: Name, Git name [prefilled], Git email, username, ssh key, signing key
	out, code := w.commit(repo, "n\nPersonal\nBrian Nguyen\nnguyenbriankhoi@gmail.com\n\n\n\n")
	want(t, code == 1, "%d %s", code, out)
	_, code = w.commit(repo, "")
	want(t, code == 0 && w.Author(repo) == meID, "author %q", w.Author(repo))
	w.check()
}

func TestUnmappedWizardPrefillsCurrentIdentity(t *testing.T) {
	w := newWorld(t, false)
	repo := w.Repo("projects/bot", true)
	_, code := w.commit(repo, "n\nMe\n\n\n\n\n\n") // Enter keeps prefilled name and email
	want(t, code == 1, "%d", code)
	p := w.cfg().Profile("Me")
	want(t, p != nil && p.Ident() == globalID, "prefill: %+v", p)
}

func TestUnmappedNewProfileRowWithExistingProfiles(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("school/15213", true)
	_, code := w.commit(repo, "n\n3\nSchool\nBrian Nguyen\nbqn@andrew.cmu.edu\n\n\n\n")
	want(t, code == 1, "%d", code)
	want(t, w.cfg().Rules["~/school/15213/"] == "School", "rules %v", w.cfg().Rules)
}

func TestGuardEOFAndBadAnswersNeverHang(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	for _, answers := range []string{"", "maybe\nwhat\n", "n\n", "n\n99\nzzz\n", "n\n3\n"} {
		_, code := w.commit(repo, answers) // B1/B2 regression: must terminate
		want(t, code == 0, "answers %q: exit %d (skips must let the commit proceed)", answers, code)
	}
	want(t, len(w.cfg().Rules) == 0, "nothing should be saved: %v", w.cfg().Rules)
	w.check()
}

func TestGuardNonInteractiveFailsOpen(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	w.Write(".no-tty-dir/x", "") // GIDS_TTY points at a missing file → no terminal
	f := filepath.Join(repo, "f.txt")
	os.WriteFile(f, []byte("ide edit\n"), 0o644)
	w.Git(repo, "add", "f.txt")
	out, err := w.TryGit(repo, "commit", "-m", "from an IDE")
	want(t, err == nil && strings.Contains(out, "no rule for ~/projects/foo"), "%v %s", err, out)
}

func TestGuardFailsOpenOnBrokenConfig(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	w.Write(".config/gids/config.yaml", "profiles: [\n")
	out, code := w.commit(repo, "")
	want(t, code == 0 && strings.Contains(out, "skipping the identity check"), "%d %s", code, out)
	r := w.Gids(w.Home, "", nil, "status")
	want(t, r.Code == 1 && strings.Contains(r.Err, "config.yaml"), "status should name the bad file: %+v", r)
}

func TestGuardHealsTamperedConfigThenAsksForRerun(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	repo := w.Repo("work/app", true)
	w.Write(".config/gids/profiles/Work.gitconfig", "[user]\n\temail = hijacked@x\n")
	out, code := w.commit(repo, "")
	want(t, code == 1 && strings.Contains(out, "repaired"), "%d %s", code, out)
	w.check()
	_, code = w.commit(repo, "")
	want(t, code == 0 && w.Author(repo) == workID, "%d %q", code, w.Author(repo))
}

// Found by the demo: git reports repo paths with symlinks resolved, so with a
// symlinked HOME (macOS /var → /private/var, or ~/code → /Volumes/…) rules
// must still be stored and shown in ~ form.
func TestSymlinkedHomeKeepsTildeRules(t *testing.T) {
	s := testenv.New(t)
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(s.Home, link); err != nil {
		t.Skip(err)
	}
	t.Setenv("HOME", link)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(link, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(link, ".gitconfig"))
	s.Write(".gitconfig", "[user]\n\tname = Global Me\n\temail = global@x.com\n")
	w := &world{s, t}
	w.MustGids(link, "install")
	w.MustGids(link, "profile", "add", "--name", "Personal", "--git-name", "Brian Nguyen", "--email", "nguyenbriankhoi@gmail.com")
	repo := w.Repo("projects/x", true)
	viaLink := filepath.Join(link, "projects", "x")
	w.MustGids(viaLink, "use", "Personal")
	want(t, w.cfg().Rules["~/projects/x/"] == "Personal", "rule stored as %v", w.cfg().Rules)
	st := w.MustGids(viaLink, "status")
	want(t, strings.Contains(st, "rule ~/projects/x/"), "status:\n%s", st)
	other := w.Repo("projects/y", true)
	out, _ := w.commit(other, "y\n\n")
	want(t, strings.Contains(out, "in ~/projects/y "), "guard should show ~ path: %s", out)
	_, code := w.commit(repo, "")
	want(t, code == 0 && w.Author(repo) == meID, "author %q", w.Author(repo))
}
