package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/config"
	"gids/internal/testenv"
)

func TestInstallUninstallRoundTripViaCLI(t *testing.T) {
	s := testenv.New(t)
	original := "[user]\n\tname = A\n\temail = a@b\n[alias]\n\tst = status"
	s.Write(".gitconfig", original)
	w := &world{s, t}
	for range 2 {
		out := w.MustGids(s.Home, "install")
		want(t, strings.Contains(out, "nstalled"), "%s", out)
	}
	w.addProfiles(config.Profile{Name: "W", GitName: "W", GitEmail: "w@x"})
	w.check()
	w.MustGids(s.Home, "uninstall")
	w.MustGids(s.Home, "uninstall")
	want(t, s.Read(s.Path(".gitconfig")) == original, "not byte-identical: %q", s.Read(s.Path(".gitconfig")))
	w.check()
}

func TestHooksPathOverriddenLaterIsReported(t *testing.T) {
	w := newWorld(t, true)
	f := w.Path(".gitconfig")
	os.WriteFile(f, []byte(w.Read(f)+"[core]\n\thooksPath = /elsewhere\n"), 0o644)
	st := w.MustGids(w.Home, "install")
	want(t, !strings.Contains(st, "won't run"), "re-install should move gids' block last: %s", st)
	want(t, strings.HasSuffix(w.Read(f), "# gids:end\n"), "block not last:\n%s", w.Read(f))
}

func TestPreviousGlobalHooksPathIsChained(t *testing.T) {
	s := testenv.New(t)
	hooks := s.Path("my-hooks")
	os.MkdirAll(hooks, 0o755)
	os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho global-hook-ran >&2\n"), 0o755)
	original := "[user]\n\tname = Global Me\n\temail = global@x.com\n[core]\n\thooksPath = ~/my-hooks\n"
	s.Write(".gitconfig", original)
	w := &world{s, t}
	w.MustGids(s.Home, "install")
	repo := w.Repo("p", true)
	out, code := w.commit(repo, "y\n\n")
	want(t, code == 0 && strings.Contains(out, "global-hook-ran"), "%d %s", code, out)
	w.MustGids(s.Home, "uninstall")
	want(t, s.Read(s.Path(".gitconfig")) == original, "uninstall didn't restore:\n%s", s.Read(s.Path(".gitconfig")))
	w.check()
}

func TestRepoHooksStillRun(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("work/app", true)
	marker := filepath.Join(repo, "ran")
	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\necho repo-hook-says-no >&2\nexit 1\n"), 0o755)
	out, code := w.commit(repo, "")
	want(t, code != 0 && strings.Contains(out, "repo-hook-says-no"), "repo hook must run and be able to block: %d %s", code, out)
	_, err := os.Stat(marker)
	want(t, err == nil, "repo hook didn't run")
	os.WriteFile(filepath.Join(repo, ".git", "hooks", "commit-msg"), []byte("#!/bin/sh\necho 'Tag: x' >> \"$1\"\n"), 0o755)
	os.Remove(hook)
	_, code = w.commit(repo, "")
	want(t, code == 0 && strings.Contains(w.Git(repo, "log", "-1", "--format=%B"), "Tag: x"), "commit-msg chain")
}

func TestMissingBinaryNeverBlocksCommits(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	hook := filepath.Join(w.GidsDir(), "hooks", "pre-commit")
	script := strings.Replace(w.Read(hook), testenv.Binary(t), "/nonexistent/gids", 1)
	os.WriteFile(hook, []byte(script), 0o755)
	out, code := w.commit(repo, "")
	want(t, code == 0 && strings.Contains(out, "not found"), "%d %s", code, out)
}

func TestHookFailsOpenWithoutHome(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	os.WriteFile(filepath.Join(repo, "f.txt"), []byte("z\n"), 0o644)
	w.Git(repo, "add", "f.txt")
	r := w.Gids(repo, "", []string{"HOME=", "XDG_CONFIG_HOME="}, "hook-exec", "pre-commit")
	want(t, r.Code == 0 && strings.Contains(r.Err, "skipping"), "hook must fail open without HOME: %+v", r)
	r = w.Gids(repo, "", []string{"HOME="}, "version")
	want(t, r.Code == 0 && strings.HasPrefix(r.Out, "gids "), "version needs nothing: %+v", r)
}

func TestHookIgnoresUnknownHookNamesAndSkipEnv(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/foo", true)
	r := w.Gids(repo, "", nil, "hook-exec", "pre-push")
	want(t, r.Code == 0, "unknown hook: %+v", r)
	r = w.Gids(repo, "", []string{"GIDS_SKIP_HOOKS=1", "GIDS_TTY=/dev/null"}, "hook-exec", "pre-commit")
	want(t, r.Code == 0 && r.Err == "", "GIDS_SKIP_HOOKS: %+v", r)
	r = w.Gids(repo, "", nil, "hook-exec", "commit-msg") // missing file arg
	want(t, r.Code == 0, "commit-msg without args: %+v", r)
}

// A hook chained to itself would exec forever and hang the commit: a repo
// hook that is a symlink to gids' own shim, or a previous global hooksPath
// that is a symlink to gids' hooks directory.
func TestHookNeverChainsToItself(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("p", true)
	shim := filepath.Join(w.GidsDir(), "hooks", "pre-commit")
	if err := os.Symlink(shim, filepath.Join(repo, ".git", "hooks", "pre-commit")); err != nil {
		t.Fatal(err)
	}
	_, code := w.commit(repo, "y\n\n") // testenv kills a hang after testenv.Timeout
	want(t, code == 0, "commit through a self-symlinked repo hook: exit %d", code)

	link := w.Path("hooks-link")
	os.Symlink(filepath.Join(w.GidsDir(), "hooks"), link)
	f := w.Path(".gitconfig")
	os.WriteFile(f, []byte("[core]\n\thooksPath = "+link+"\n"+w.Read(f)), 0o644)
	w.MustGids(w.Home, "install") // re-renders hooks with the new previous hooksPath
	want(t, !strings.Contains(w.Read(shim), "hooks-link"), "chains to its own (symlinked) hooks dir:\n%s", w.Read(shim))
	_, code = w.commit(repo, "y\n\n")
	want(t, code == 0, "commit with a symlinked previous hooksPath: exit %d", code)
	w.check()
}
