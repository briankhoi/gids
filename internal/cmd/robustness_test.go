package cmd_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gids/internal/app"
	"gids/internal/config"
	"gids/internal/invariant"
	"gids/internal/render"
	"gids/internal/testenv"
)

// strictNow checks what must hold even before any self-heal: config.yaml is
// valid (atomic writes) and the ~/.gitconfig block is well-formed.
func strictNow(t *testing.T, w *world) {
	t.Helper()
	if _, err := config.Load(filepath.Join(w.GidsDir(), "config.yaml")); err != nil {
		t.Fatalf("I1 broken right after a kill: %v", err)
	}
	if _, err := render.HasBlock(w.Read(w.Path(".gitconfig"))); err != nil {
		t.Fatalf("I3 broken right after a kill: %v", err)
	}
}

// ---------------- model-based random sequences ----------------

// model is a deliberately simple shadow implementation of gids' state.
type model struct {
	profiles  map[string]string // name -> email
	rules     map[string]string // "~/dir/" -> profile
	installed bool
}

func (m *model) fold(name string) (string, bool) {
	for n := range m.profiles {
		if strings.EqualFold(n, name) {
			return n, true
		}
	}
	return "", false
}

// expectedEmail is the identity git should apply in repo dir (relative to
// home), per the model: the deepest rule containing it (depth = number of
// path segments, so "~/" is shallower than "~/a/"), else the global one.
func (m *model) expectedEmail(dir string) string {
	if !m.installed {
		return "global@x.com"
	}
	best, bestDepth := "", -1
	for r, p := range m.rules {
		rd := strings.Trim(strings.TrimPrefix(r, "~/"), "/")
		depth := 0
		if rd != "" {
			depth = strings.Count(rd, "/") + 1
		}
		if (rd == "" || dir == rd || strings.HasPrefix(dir, rd+"/")) && depth > bestDepth {
			best, bestDepth = p, depth
		}
	}
	if best == "" {
		return "global@x.com"
	}
	return m.profiles[best]
}

func TestRandomSequencesMatchModelAndGit(t *testing.T) {
	seeds := []int64{1, 2, 3, 4}
	if testing.Short() {
		seeds = seeds[:1]
	}
	names := []string{"Work", "work", "Personal", "School", "bad name", "x"}
	dirs := []string{"a", "a/b", "a/b/c", "d", "."}
	repos := []string{"a/r1", "a/b/r2", "a/b/c/r3", "d/r4", "e/r5"}
	for _, seed := range seeds {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			w := newWorld(t, false)
			for _, r := range repos {
				w.Repo(r, true)
			}
			m := &model{profiles: map[string]string{}, rules: map[string]string{}, installed: true}
			for step := range 60 {
				var desc string
				switch op := rng.Intn(7); op {
				case 0, 1: // add profile (sometimes invalid / duplicate)
					n := names[rng.Intn(len(names))]
					email := fmt.Sprintf("%s%d@x", strings.ToLower(strings.ReplaceAll(n, " ", "")), step)
					if rng.Intn(5) == 0 {
						email = "invalid"
					}
					r := w.Gids(w.Home, "", nil, "profile", "add", "--name", n, "--git-name", "N", "--email", email)
					_, exists := m.fold(n)
					ok := !exists && config.ValidateProfileName(n) == nil && email != "invalid"
					if ok != (r.Code == 0) {
						t.Fatalf("step %d add %q %q: exit %d, model ok=%v\n%s", step, n, email, r.Code, ok, r.Err)
					}
					if ok {
						m.profiles[n] = email
					}
					desc = "add " + n
				case 2: // rule add
					d, n := dirs[rng.Intn(len(dirs))], names[rng.Intn(len(names))]
					r := w.Gids(w.Home, "", nil, "rule", "add", d, n)
					real, ok := m.fold(n)
					if ok != (r.Code == 0) {
						t.Fatalf("step %d rule add %s %s: exit %d model %v\n%s", step, d, n, r.Code, ok, r.Err)
					}
					if ok {
						key := "~/" + d + "/"
						if d == "." {
							key = "~/"
						}
						m.rules[key] = real
					}
					desc = "rule " + d + "→" + n
				case 3: // rule remove
					d := dirs[rng.Intn(len(dirs))]
					key := "~/" + d + "/"
					if d == "." {
						key = "~/"
					}
					r := w.Gids(w.Home, "", nil, "rule", "remove", d)
					_, ok := m.rules[key]
					if ok != (r.Code == 0) {
						t.Fatalf("step %d rule remove %s: exit %d model %v", step, d, r.Code, ok)
					}
					delete(m.rules, key)
					desc = "unrule " + d
				case 4: // delete profile --force
					n := names[rng.Intn(len(names))]
					r := w.Gids(w.Home, "", nil, "profile", "delete", n, "--force")
					real, ok := m.fold(n)
					if ok != (r.Code == 0) {
						t.Fatalf("step %d delete %s: exit %d model %v", step, n, r.Code, ok)
					}
					if ok {
						delete(m.profiles, real)
						for k, v := range m.rules {
							if v == real {
								delete(m.rules, k)
							}
						}
					}
					desc = "delete " + n
				case 5: // rename
					from, to := names[rng.Intn(len(names))], names[rng.Intn(len(names))]
					r := w.Gids(w.Home, "", nil, "profile", "edit", from, "--name", to)
					real, ok := m.fold(from)
					clash, taken := m.fold(to)
					valid := ok && config.ValidateProfileName(to) == nil && (!taken || clash == real)
					if valid != (r.Code == 0) {
						t.Fatalf("step %d rename %s→%s: exit %d model %v\n%s", step, from, to, r.Code, valid, r.Err)
					}
					if valid {
						m.profiles[to] = m.profiles[real]
						if to != real {
							delete(m.profiles, real)
						}
						for k, v := range m.rules {
							if v == real {
								m.rules[k] = to
							}
						}
					}
					desc = "rename " + from + "→" + to
				case 6: // install / uninstall
					if rng.Intn(2) == 0 {
						w.MustGids(w.Home, "install")
						m.installed = true
						desc = "install"
					} else {
						w.MustGids(w.Home, "uninstall")
						m.installed = false
						desc = "uninstall"
					}
				}
				w.check()
				// gids' state equals the model…
				cfg := w.cfg()
				got := map[string]string{}
				for _, p := range cfg.Profiles {
					got[p.Name] = p.GitEmail
				}
				if fmt.Sprint(sorted(got)) != fmt.Sprint(sorted(m.profiles)) || fmt.Sprint(sorted(cfg.Rules)) != fmt.Sprint(sorted(m.rules)) {
					t.Fatalf("step %d (%s): state diverged\n got %v %v\nwant %v %v", step, desc, got, cfg.Rules, m.profiles, m.rules)
				}
				// …and so does what real git resolves in every repo (differential check of includeIf).
				for _, r := range repos {
					email := w.Git(w.Path(r), "config", "user.email")
					if exp := m.expectedEmail(r); email != exp {
						t.Fatalf("step %d (%s): git uses %q in %s, model expects %q\nrules %v", step, desc, email, r, exp, m.rules)
					}
				}
			}
		})
	}
}

func sorted(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// Ensure the invariant helper itself catches violations (a checker that
// never fails proves nothing).
func TestInvariantCheckerDetectsViolations(t *testing.T) {
	w := newWorld(t, true)
	env, _ := app.NewEnv()
	env.Exe = testenv.Binary(t)
	mustFail := func(what string) {
		t.Helper()
		if invariant.Check(env) == nil {
			t.Fatalf("checker missed: %s", what)
		}
		w.Gids(w.Home, "", nil, "status") // heal
		if err := invariant.Check(env); err != nil && !strings.Contains(what, "config") {
			t.Fatalf("heal didn't fix %s: %v", what, err)
		}
	}
	os.WriteFile(filepath.Join(w.GidsDir(), "profiles", "Stray.gitconfig"), []byte("x"), 0o644)
	mustFail("stray file")
	os.WriteFile(filepath.Join(w.GidsDir(), "profiles", "Work.gitconfig"), []byte("x"), 0o644)
	mustFail("tampered file")
	os.Chmod(filepath.Join(w.GidsDir(), "hooks", "pre-commit"), 0o644)
	mustFail("non-executable hook")
	os.WriteFile(filepath.Join(w.GidsDir(), "hooks", ".gids-tmp-pre-commit-1"), []byte("x"), 0o644)
	mustFail("temp file")
	os.WriteFile(filepath.Join(w.GidsDir(), "config.yaml"), []byte("profiles: [\n"), 0o600)
	if invariant.Check(env) == nil {
		t.Fatal("checker missed invalid config")
	}
}
