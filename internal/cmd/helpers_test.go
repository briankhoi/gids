package cmd_test

import (
	"path/filepath"
	"testing"

	"gids/internal/app"
	"gids/internal/config"
	"gids/internal/invariant"
	"gids/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

const (
	globalID = "Global Me <global@x.com>"
	workID   = "Work Me <work@co.com>"
	meID     = "Brian Nguyen <nguyenbriankhoi@gmail.com>"
)

// world is a sandbox with gids installed, a global identity, and two profiles.
type world struct {
	*testenv.Sandbox
	t *testing.T
}

func newWorld(t *testing.T, profiles bool) *world {
	t.Helper()
	s := testenv.New(t)
	s.Write(".gitconfig", "[user]\n\tname = Global Me\n\temail = global@x.com\n")
	w := &world{s, t}
	w.MustGids(s.Home, "install")
	if profiles {
		w.addProfiles(
			config.Profile{Name: "Work", GitName: "Work Me", GitEmail: "work@co.com"},
			config.Profile{Name: "Personal", GitName: "Brian Nguyen", GitEmail: "nguyenbriankhoi@gmail.com"},
		)
	}
	return w
}

// addProfiles saves profiles through the API (and heals, like any command),
// so fixtures don't depend on the profile command.
func (w *world) addProfiles(ps ...config.Profile) {
	w.t.Helper()
	env, err := app.NewEnv()
	if err != nil {
		w.t.Fatal(err)
	}
	env.Exe = testenv.Binary(w.t)
	if _, err := env.Mutate(func(c *config.Config) error {
		for _, p := range ps {
			if err := c.AddProfile(p); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		w.t.Fatal(err)
	}
}

// check asserts every SPEC §11 invariant holds right now.
func (w *world) check() {
	w.t.Helper()
	env, err := app.NewEnv()
	if err != nil {
		w.t.Fatal(err)
	}
	env.Exe = testenv.Binary(w.t)
	if err := invariant.Check(env); err != nil {
		w.t.Fatalf("invariants violated:\n%v", err)
	}
}

func (w *world) cfg() *config.Config {
	w.t.Helper()
	c, err := config.Load(filepath.Join(w.GidsDir(), "config.yaml"))
	if err != nil {
		w.t.Fatal(err)
	}
	return c
}

func (w *world) commit(dir, answers string, args ...string) (string, int) {
	w.t.Helper()
	if len(args) == 0 {
		args = []string{"-m", "msg"}
	}
	return w.GitCommit(dir, answers, args...)
}

func want(t *testing.T, cond bool, format string, a ...any) {
	t.Helper()
	if !cond {
		t.Fatalf(format, a...)
	}
}
