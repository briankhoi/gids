package cmd_test

// Deterministic verification (SPEC §11, NOTES "Verification project"):
//   - TestCrashAtEveryStep: kill gids at every single file/git step of every
//     mutating command, not at sampled moments.
//   - TestEveryReachableState: breadth-first search over every state a small
//     domain can reach, trying every operation from every state against a
//     model and real git.
//   - TestTracesFollowTheModeledProtocol: the real code's step order matches
//     what the TLA+ models (verify/tla) assume.

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gids/internal/testenv"
)

// scenario is one mutating command run from a prepared world.
type scenario struct {
	name  string
	stdin string
	setup func(t *testing.T) *world
	run   func(w *world) (dir string, args []string)
	check func(t *testing.T, w *world) // extra checks after a crash + heal
}

func installedWorld(t *testing.T) *world {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	w.Repo("work/app", true)
	return w
}

func amendWorld(t *testing.T) *world {
	w := installedWorld(t)
	repo := w.Repo("bot", true)
	w.Git(repo, "checkout", "-q", "-b", "feat")
	for i := range 3 {
		w.Commit(repo, fmt.Sprintf("c%d", i), "Brian Nguyen <bqn@andrew.cmu.edu>")
	}
	return w
}

// amendIntact: the branch is either untouched or fully rewritten (I6), and
// the repo is sound.
func amendIntact(t *testing.T, w *world) {
	t.Helper()
	repo := w.Path("bot")
	emails := strings.Fields(w.Git(repo, "log", "--format=%ae", "main..feat"))
	school := strings.Count(strings.Join(emails, " "), "andrew")
	if len(emails) != 3 || (school != 0 && school != 3) {
		t.Fatalf("partial history after a crash: %v", emails)
	}
	if s := w.Git(repo, "status", "--porcelain"); s != "" {
		t.Fatalf("worktree changed: %s", s)
	}
	w.Git(repo, "fsck", "--no-progress")
}

func scenarios() []scenario {
	home := func(args ...string) func(w *world) (string, []string) {
		return func(w *world) (string, []string) { return w.Home, args }
	}
	return []scenario{
		{name: "install", setup: func(t *testing.T) *world {
			w := installedWorld(t)
			w.MustGids(w.Home, "uninstall")
			return w
		}, run: home("install")},
		{name: "uninstall", setup: installedWorld, run: home("uninstall")},
		{name: "profile add", setup: installedWorld, run: home("profile", "add", "--name", "New", "--git-name", "N", "--email", "n@x")},
		{name: "profile rename", setup: installedWorld, run: home("profile", "edit", "Work", "--name", "Job")},
		{name: "profile delete --force", setup: installedWorld, run: home("profile", "delete", "Work", "--force")},
		{name: "rule add", setup: installedWorld, run: home("rule", "add", "~/other", "Personal")},
		{name: "rule remove", setup: installedWorld, run: home("rule", "remove", "~/work")},
		{name: "use", stdin: "y\n", setup: installedWorld, run: func(w *world) (string, []string) {
			return w.Path("work", "app"), []string{"use", "Personal"}
		}},
		{name: "heal after tampering", setup: func(t *testing.T) *world {
			w := installedWorld(t)
			os.WriteFile(filepath.Join(w.GidsDir(), "profiles", "Work.gitconfig"), []byte("x"), 0o644)
			os.WriteFile(filepath.Join(w.GidsDir(), "profiles", "Ghost.gitconfig"), []byte("x"), 0o644)
			os.Remove(filepath.Join(w.GidsDir(), "hooks", "pre-commit"))
			return w
		}, run: home("status")},
		{name: "amend", setup: amendWorld, run: func(w *world) (string, []string) {
			return w.Path("bot"), []string{"amend", "--since", "main", "--author", "Personal", "-y"}
		}, check: amendIntact},
	}
}

// TestCrashAtEveryStep kills each mutating command at every one of its
// steps (testhook.Point: each file operation, lock, and git command), then
// checks what must hold immediately (I1, I3) and, after the next gids
// command self-heals, everything (I1–I4; I6 for amend). Unlike timed kills
// this is exhaustive: every crash point is hit exactly once.
func TestCrashAtEveryStep(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	total := 0
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			base := snapshot(t, sc.setup(t), t.TempDir())
			w := restore(t, base)
			dir, args := sc.run(w)
			steps, r := traceOf(t, w, dir, sc.stdin, args...)
			if r.Code != 0 || len(steps) == 0 {
				t.Fatalf("uninterrupted run: exit %d, %d steps\n%s", r.Code, len(steps), r.Err)
			}
			for k := 1; k <= len(steps); k++ {
				w := restore(t, base)
				dir, args := sc.run(w)
				r := w.Gids(dir, sc.stdin, []string{"GIDS_TEST_CRASH_AT=" + strconv.Itoa(k)}, args...)
				if r.Code != -1 {
					t.Fatalf("step %d/%d (%v) didn't crash: exit %d (non-deterministic step count?)", k, len(steps), steps[k-1], r.Code)
				}
				strictNow(t, w)
				w.Gids(w.Home, "", nil, "status") // any invocation self-heals
				w.check()
				if sc.check != nil {
					sc.check(t, w)
				}
			}
			total += len(steps)
			t.Logf("%d crash points, all recovered", len(steps))
		})
	}
	t.Logf("%d crash points in total", total)
}

// ---------------- exhaustive state-space search ----------------

// bfsOp is one concrete operation of the bounded domain and its effect on
// the model (returning whether the model says it succeeds).
type bfsOp struct {
	name  string
	args  []string
	apply func(m *model) bool
}

func (m *model) clone() *model {
	return &model{profiles: maps.Clone(m.profiles), rules: maps.Clone(m.rules), installed: m.installed}
}

func (m *model) key() string {
	return fmt.Sprint(sorted(m.profiles), sorted(m.rules), m.installed)
}

func bfsOps() []bfsOp {
	add := func(name, email string) bfsOp {
		return bfsOp{"add " + name, []string{"profile", "add", "--name", name, "--git-name", "N", "--email", email}, func(m *model) bool {
			if _, taken := m.fold(name); taken || strings.Contains(name, " ") {
				return false
			}
			m.profiles[name] = email
			return true
		}}
	}
	rule := func(dir, profile string) bfsOp {
		key := "~/" + dir + "/"
		if dir == "." {
			key = "~/"
		}
		return bfsOp{"rule " + dir + "→" + profile, []string{"rule", "add", dir, profile}, func(m *model) bool {
			real, ok := m.fold(profile)
			if ok {
				m.rules[key] = real
			}
			return ok
		}}
	}
	return []bfsOp{
		add("Work", "work@x"), add("Personal", "me@x"), add("work", "dup@x"), add("bad name", "b@x"),
		rule("a", "Work"), rule("a/b", "Personal"), rule(".", "Work"),
		{"unrule a", []string{"rule", "remove", "a"}, func(m *model) bool {
			_, ok := m.rules["~/a/"]
			delete(m.rules, "~/a/")
			return ok
		}},
		{"delete Work", []string{"profile", "delete", "Work", "--force"}, func(m *model) bool {
			real, ok := m.fold("Work")
			if ok {
				delete(m.profiles, real)
				maps.DeleteFunc(m.rules, func(_, v string) bool { return v == real })
			}
			return ok
		}},
		{"rename Work→Job", []string{"profile", "edit", "Work", "--name", "Job"}, func(m *model) bool {
			real, ok := m.fold("Work")
			if _, taken := m.fold("Job"); !ok || taken {
				return false
			}
			m.profiles["Job"] = m.profiles[real]
			delete(m.profiles, real)
			for k, v := range m.rules {
				if v == real {
					m.rules[k] = "Job"
				}
			}
			return true
		}},
		{"install", []string{"install"}, func(m *model) bool { m.installed = true; return true }},
		{"uninstall", []string{"uninstall"}, func(m *model) bool { m.installed = false; return true }},
	}
}

// TestEveryReachableState explores, breadth first, every state the domain
// (bfsOps) can reach from a fresh install, and from each state tries every
// operation: gids' exit status, its saved state and the invariants must
// match the model, and in every newly reached state real git must resolve
// the identity the model predicts in every repo. Within this domain the
// search is complete: no reachable state or transition is skipped.
func TestEveryReachableState(t *testing.T) {
	if os.Getenv("GIDS_VERIFY") != "1" {
		t.Skip("exhaustive (~5 min): run with `make verify` (GIDS_VERIFY=1)")
	}
	repos := []string{"a/r1", "a/b/r2", "a/b/c/r3", "d/r4"}
	w0 := newWorld(t, false)
	for _, r := range repos {
		w0.Repo(r, true)
	}
	snaps := t.TempDir()
	type node struct {
		m    *model
		snap string
	}
	start := &model{profiles: map[string]string{}, rules: map[string]string{}, installed: true}
	queue := []node{{start, snapshot(t, w0, snaps)}}
	seen := map[string]bool{start.key(): true}
	transitions := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, op := range bfsOps() {
			transitions++
			want := n.m.clone()
			ok := op.apply(want)
			t.Run("", func(t *testing.T) {
				w := restore(t, n.snap)
				r := w.Gids(w.Home, "", nil, op.args...)
				if ok != (r.Code == 0) {
					t.Fatalf("from %s, %s: exit %d, model says ok=%v\n%s", n.m.key(), op.name, r.Code, ok, r.Err)
				}
				w.check()
				got := map[string]string{}
				for _, p := range w.cfg().Profiles {
					got[p.Name] = p.GitEmail
				}
				if g := fmt.Sprint(sorted(got), sorted(w.cfg().Rules)); g != fmt.Sprint(sorted(want.profiles), sorted(want.rules)) {
					t.Fatalf("from %s, %s: state %s, model %s", n.m.key(), op.name, g, want.key())
				}
				if seen[want.key()] {
					return
				}
				seen[want.key()] = true
				for _, repo := range repos { // differential check against real git
					if email := w.Git(w.Path(repo), "config", "user.email"); email != want.expectedEmail(repo) {
						t.Fatalf("state %s: git uses %q in %s, model expects %q", want.key(), email, repo, want.expectedEmail(repo))
					}
				}
				queue = append(queue, node{want, snapshot(t, w, snaps)})
			})
			if t.Failed() {
				return
			}
		}
	}
	t.Logf("explored %d reachable states and %d transitions (every op from every state)", len(seen), transitions)
}

// ---------------- trace conformance with the TLA+ models ----------------

// TestTracesFollowTheModeledProtocol checks, on the real code's step trace,
// the properties verify/tla/GidsFiles.tla assumes of every command:
//
//	P1 every file is replaced by temp-file + rename (create, write, rename)
//	P2 every change to gids' files or the global git config happens while
//	   holding the lock
//	P3 install activates last: the ~/.gitconfig rename follows every
//	   generated-file change in that lock region
//	P4 uninstall deactivates first: the ~/.gitconfig change precedes every
//	   generated-file removal in that lock region
//	P5 a config change is saved before the generated files are synced
//	P6 amend moves the branch exactly once, last, with compare-and-swap
func TestTracesFollowTheModeledProtocol(t *testing.T) {
	mutating := map[string]bool{"mkdir": true, "create-temp": true, "write-temp": true, "rename": true, "remove": true, "chmod": true, "rmdir": true}
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			w := sc.setup(t)
			dir, args := sc.run(w)
			steps, r := traceOf(t, w, dir, sc.stdin, args...)
			if r.Code != 0 {
				t.Fatalf("exit %d: %s", r.Code, r.Err)
			}
			gids, global := w.GidsDir(), w.Path(".gitconfig")
			ours := func(p string) bool {
				return p == global || p == gids || strings.HasPrefix(p, gids+string(filepath.Separator))
			}
			held := false
			var region []testenv.Step // steps of the current lock region
			pending := map[string][]string{}
			checkRegion := func() {
				globalAt, firstGen, lastGen, cfgAt, firstSync := -1, -1, -1, -1, -1
				for i, s := range region {
					switch {
					case !mutating[s.Op]:
					case s.Arg == global && (s.Op == "rename" || s.Op == "remove"):
						globalAt = i
					case strings.HasSuffix(s.Arg, "config.yaml"):
						if s.Op == "rename" {
							cfgAt = i
						}
					case ours(s.Arg) && s.Arg != global && s.Op != "mkdir":
						if firstGen < 0 {
							firstGen = i
						}
						lastGen = i
						if firstSync < 0 {
							firstSync = i
						}
					}
				}
				switch sc.name {
				case "install":
					if globalAt >= 0 && lastGen > globalAt {
						t.Errorf("P3: generated file changed after activation:\n%v", region)
					}
				case "uninstall":
					if globalAt >= 0 && firstGen >= 0 && firstGen < globalAt {
						t.Errorf("P4: generated file removed before deactivation:\n%v", region)
					}
				}
				if cfgAt >= 0 && firstSync >= 0 && firstSync < cfgAt {
					t.Errorf("P5: generated files synced before config.yaml was saved:\n%v", region)
				}
			}
			var gitMoves []string
			for _, s := range steps {
				switch {
				case s.Op == "lock":
					held, region = true, nil
					continue
				case s.Op == "unlock":
					checkRegion()
					held = false
					continue
				case s.Op == "git" && strings.HasPrefix(s.Arg, "update-ref"):
					gitMoves = append(gitMoves, s.Arg)
				}
				if held {
					region = append(region, s)
				}
				if !mutating[s.Op] || !ours(s.Arg) {
					continue
				}
				if !held && s.Op != "mkdir" {
					t.Errorf("P2: %s %s outside the lock", s.Op, s.Arg)
				}
				switch s.Op {
				case "create-temp", "write-temp":
					pending[s.Arg] = append(pending[s.Arg], s.Op)
				case "rename":
					if got := strings.Join(pending[s.Arg], ","); got != "create-temp,write-temp" {
						t.Errorf("P1: %s renamed into place after %q, not create-temp,write-temp", s.Arg, got)
					}
					delete(pending, s.Arg)
				}
			}
			if held {
				t.Error("P2: exited holding the lock (in the trace)")
			}
			if sc.name == "amend" {
				// backup ref, then the branch: last, and with an expected old value (CAS).
				if len(gitMoves) != 2 || !strings.Contains(gitMoves[0], "refs/gids/amend-backup") ||
					len(strings.Fields(gitMoves[1])) < 5 || !strings.Contains(gitMoves[1], "refs/heads/feat") {
					t.Errorf("P6: ref updates %q", gitMoves)
				}
				for i, s := range steps {
					if s.Op == "git" && s.Arg == gitMoves[len(gitMoves)-1] && i != lastIndexMutatingGit(steps) {
						t.Errorf("P6: the branch update isn't amend's last change")
					}
				}
			}
		})
	}
}

func lastIndexMutatingGit(steps []testenv.Step) int {
	last := -1
	for i, s := range steps {
		if s.Op == "git" && (strings.HasPrefix(s.Arg, "update-ref") || strings.HasPrefix(s.Arg, "commit-tree")) {
			last = i
		}
	}
	return last
}
