package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/app"
	"gids/internal/config"
	"gids/internal/invariant"
	"gids/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func newEnv(t *testing.T) (*testenv.Sandbox, *app.Env) {
	s := testenv.New(t)
	env, err := app.NewEnv()
	if err != nil {
		t.Fatal(err)
	}
	env.Exe = "/opt/gids/bin/gids"
	return s, env
}

func check(t *testing.T, env *app.Env) {
	t.Helper()
	if err := invariant.Check(env); err != nil {
		t.Fatalf("invariants violated:\n%v", err)
	}
}

func addWork(t *testing.T, env *app.Env) {
	t.Helper()
	if _, err := env.Mutate(func(c *config.Config) error {
		if err := c.AddProfile(config.Profile{Name: "Work", GitName: "W", GitEmail: "w@co"}); err != nil {
			return err
		}
		c.SetRule("~/work/", "Work")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInstallUninstallRestoresEverythingByteForByte(t *testing.T) {
	for name, original := range map[string]*string{
		"no gitconfig":       nil,
		"normal":             new("[user]\n\tname = Me\n\temail = me@x\n"),
		"no final newline":   new("[user]\n\tname = Me"),
		"existing hooksPath": new("[core]\n\thooksPath = ~/my-hooks\n"),
	} {
		t.Run(name, func(t *testing.T) {
			s, env := newEnv(t)
			gc := s.Path(".gitconfig")
			if original != nil {
				os.WriteFile(gc, []byte(*original), 0o600)
			}
			addWork(t, env)
			check(t, env)
			for range 2 { // idempotent
				if _, err := env.Install(); err != nil {
					t.Fatal(err)
				}
				check(t, env)
			}
			if prev, _ := env.PrevHooksPath(); name == "existing hooksPath" && prev != s.Path("my-hooks") {
				t.Fatalf("previous hooksPath = %q", prev)
			}
			hook := s.Read(filepath.Join(s.GidsDir(), "hooks", "pre-commit"))
			if name == "existing hooksPath" && !strings.Contains(hook, s.Path("my-hooks")) {
				t.Fatalf("hook doesn't chain to the previous hooksPath:\n%s", hook)
			}
			for range 2 {
				if _, err := env.Uninstall(); err != nil {
					t.Fatal(err)
				}
				check(t, env)
			}
			b, err := os.ReadFile(gc)
			if original == nil {
				if err == nil {
					t.Fatalf("gids-created ~/.gitconfig left behind: %q", b)
				}
			} else if string(b) != *original {
				t.Fatalf("not restored:\n got %q\nwant %q", b, *original)
			}
			if fi, _ := os.Stat(gc); original != nil && fi.Mode().Perm() != 0o600 {
				t.Fatalf("permissions changed to %v", fi.Mode().Perm())
			}
			if _, err := os.Stat(env.Paths.Config); err != nil {
				t.Fatal("uninstall must keep config.yaml")
			}
		})
	}
}

//go:fix inline
func ptr(s string) *string { return new(s) }

func TestHealRepairsTamperingAndStrays(t *testing.T) {
	s, env := newEnv(t)
	addWork(t, env)
	if _, err := env.Install(); err != nil {
		t.Fatal(err)
	}
	dir := s.GidsDir()
	os.WriteFile(filepath.Join(dir, "profiles", "Work.gitconfig"), []byte("tampered"), 0o644)
	os.WriteFile(filepath.Join(dir, "profiles", "Ghost.gitconfig"), []byte("stray"), 0o644)
	os.WriteFile(filepath.Join(dir, "profiles", ".gids-tmp-x-123"), []byte("partial"), 0o644)
	os.Remove(filepath.Join(dir, "hooks", "post-commit"))
	os.Chmod(filepath.Join(dir, "hooks", "pre-commit"), 0o644)
	cfg, _ := env.Load()
	changed, identity, err := env.Heal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !identity || len(changed) < 3 {
		t.Fatalf("changed=%v identity=%v", changed, identity)
	}
	check(t, env)
	if changed, identity, _ := env.Heal(cfg); len(changed) != 0 || identity {
		t.Fatalf("second heal changed %v", changed)
	}
	// Hook-only repairs don't count as identity changes.
	os.WriteFile(filepath.Join(dir, "hooks", "commit-msg"), []byte("x"), 0o755)
	if _, identity, _ := env.Heal(cfg); identity {
		t.Fatal("hook repair flagged as identity change")
	}
}

// Only names gids generates are ever deleted (render.Managed): a file someone
// else put in gids' directories survives heal and uninstall.
func TestForeignFilesAreNeverDeleted(t *testing.T) {
	s, env := newEnv(t)
	addWork(t, env)
	if _, err := env.Install(); err != nil {
		t.Fatal(err)
	}
	dir := s.GidsDir()
	foreign := []string{"README.md", ".DS_Store", "profiles/.DS_Store", "profiles/notes.txt", "hooks/.DS_Store", "hooks/my-own-hook"}
	for _, f := range foreign {
		os.WriteFile(filepath.Join(dir, filepath.FromSlash(f)), []byte("mine"), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "profiles", "Ghost.gitconfig"), []byte("stray"), 0o644) // gids-shaped: removed
	cfg, _ := env.Load()
	if _, _, err := env.Heal(cfg); err != nil {
		t.Fatal(err)
	}
	check(t, env)
	if _, err := os.Stat(filepath.Join(dir, "profiles", "Ghost.gitconfig")); !os.IsNotExist(err) {
		t.Error("gids-shaped stray survived heal")
	}
	if _, err := env.Uninstall(); err != nil {
		t.Fatal(err)
	}
	check(t, env)
	for _, f := range foreign {
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f))); err != nil || string(b) != "mine" {
			t.Errorf("foreign file %s was touched: %q %v", f, b, err)
		}
	}
}

func TestHealRemovesGeneratedFilesWhenNotInstalled(t *testing.T) {
	s, env := newEnv(t)
	addWork(t, env)
	env.Install()
	// Simulate a crash during uninstall: block removed, files left.
	os.WriteFile(s.Path(".gitconfig"), nil, 0o644)
	cfg, _ := env.Load()
	if _, _, err := env.Heal(cfg); err != nil {
		t.Fatal(err)
	}
	check(t, env)
}

func TestInstallRefusesDamagedBlockAndChangesNothing(t *testing.T) {
	s, env := newEnv(t)
	damaged := "[user]\n# gids:begin (managed by `gids install`; remove with `gids uninstall`)\n[include]\n"
	gc := s.Write(".gitconfig", damaged)
	if _, err := env.Install(); err == nil {
		t.Fatal("install accepted a damaged block")
	}
	if _, err := env.Uninstall(); err == nil {
		t.Fatal("uninstall accepted a damaged block")
	}
	if s.Read(gc) != damaged {
		t.Fatal("damaged file was modified")
	}
}

func TestInstallThroughSymlinkedGitconfig(t *testing.T) {
	s, env := newEnv(t)
	target := s.Write("dotfiles/gitconfig", "[user]\n\tname = Me\n")
	if err := os.Symlink(target, s.Path(".gitconfig")); err != nil {
		t.Skip(err)
	}
	if _, err := env.Install(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(s.Path(".gitconfig")); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
	check(t, env)
	env.Uninstall()
	if s.Read(target) != "[user]\n\tname = Me\n" {
		t.Fatalf("target not restored: %q", s.Read(target))
	}
}

func TestMutateRejectsInvalidAndKeepsOldConfig(t *testing.T) {
	_, env := newEnv(t)
	addWork(t, env)
	before, _ := os.ReadFile(env.Paths.Config)
	_, err := env.Mutate(func(c *config.Config) error {
		c.SetRule("~/x/", "NoSuchProfile")
		return nil
	})
	if err == nil {
		t.Fatal("invalid mutation saved")
	}
	after, _ := os.ReadFile(env.Paths.Config)
	if string(before) != string(after) {
		t.Fatal("config changed after a rejected mutation")
	}
}
