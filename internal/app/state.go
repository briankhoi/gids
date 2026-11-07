// Package app is gids' core: it keeps the generated files and the
// ~/.gitconfig block consistent with config.yaml (SPEC §11 invariants).
// Commands go through it for every change.
package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gids/internal/config"
	"gids/internal/fsx"
	"gids/internal/git"
	"gids/internal/render"
)

// MinGit is the oldest git gids supports (commit --trailer, --path-format).
var MinGit = [2]int{2, 32}

// Env is the resolved environment gids operates on.
type Env struct {
	Paths config.Paths
	Exe   string // gids binary baked into hooks
}

// NewEnv resolves paths and the running executable.
func NewEnv() (*Env, error) {
	p, err := config.ResolvePaths()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("can't find the gids executable: %w", err)
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return &Env{Paths: p, Exe: exe}, nil
}

func (e *Env) git() git.Client { return git.Client{Dir: e.Paths.Home} }

// Load reads config.yaml.
func (e *Env) Load() (*config.Config, error) { return config.Load(e.Paths.Config) }

// readGlobal returns the global git config's contents and whether it exists.
func (e *Env) readGlobal() (content string, exists bool, err error) {
	target, err := fsx.Target(e.Paths.GlobalGitconfig)
	if err != nil {
		return "", false, err
	}
	b, err := os.ReadFile(target)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	return string(b), err == nil, err
}

// Installed reports whether the ~/.gitconfig block is present.
func (e *Env) Installed() (bool, error) {
	c, _, err := e.readGlobal()
	if err != nil {
		return false, err
	}
	return render.HasBlock(c)
}

// display returns p as "~/…" when inside home, '/'-separated, for git config
// values that git expands (include.path, core.hooksPath) and for messages.
func (e *Env) display(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	for _, h := range config.HomeForms(e.Paths.Home) {
		if rest, ok := strings.CutPrefix(p, h+"/"); ok {
			return "~/" + rest
		}
	}
	return p
}

// Display is display for callers outside the package.
func (e *Env) Display(p string) string { return e.display(p) }

// Expand replaces a leading "~/" with the home directory.
func (e *Env) Expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(e.Paths.Home, p[2:])
	}
	return p
}

// PrevHooksPath is the core.hooksPath that would apply without gids: the last
// global value that isn't gids' own, else the system value, else "".
func (e *Env) PrevHooksPath() (string, error) {
	g := e.git()
	vals, err := g.ConfigGetAll("core.hooksPath", "--global")
	if err != nil {
		return "", err
	}
	for _, val := range slices.Backward(vals) {
		if !sameDir(e.Expand(val), e.Paths.HooksDir) {
			return e.Expand(val), nil
		}
	}
	v, _, err := g.ConfigGet("core.hooksPath", "--system")
	return e.Expand(v), err
}

// sameDir reports whether a and b name the same directory, also through
// symlinks (a hooksPath symlinked to gids' own hooks must not be chained to).
func sameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	fa, errA := os.Stat(a)
	fb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(fa, fb)
}

// desired is the generated file set that should exist right now.
func (e *Env) desired(cfg *config.Config) ([]render.File, error) {
	installed, err := e.Installed()
	if err != nil || !installed {
		return nil, err
	}
	prev, err := e.PrevHooksPath()
	if err != nil {
		return nil, err
	}
	return render.Render(cfg, render.Input{Home: e.Paths.Home, Exe: e.Exe, PrevHooksPath: prev}), nil
}

// Heal makes the generated files exactly equal to what config.yaml implies
// (SPEC §7.2 self-healing, invariant I2). It returns the relative paths it
// changed; identity reports whether any of them affects which identity git
// picks (as opposed to just the hook scripts).
func (e *Env) Heal(cfg *config.Config) (changed []string, identity bool, err error) {
	files, err := e.desired(cfg)
	if err != nil {
		return nil, false, err
	}
	changed, err = e.sync(files)
	for _, c := range changed {
		if !strings.HasPrefix(c, "hooks/") {
			identity = true
		}
	}
	return changed, identity, err
}

// sync writes files and deletes every other gids-generated file (including
// temp files left by a killed process).
func (e *Env) sync(files []render.File) ([]string, error) {
	dir := e.Paths.Dir
	want := map[string]bool{}
	var changed []string
	for _, f := range files {
		want[f.Rel] = true
		file := filepath.Join(dir, filepath.FromSlash(f.Rel))
		perm := fs.FileMode(0o644)
		if f.Exec {
			perm = 0o755
		}
		cur, err := os.ReadFile(file)
		fi, statErr := os.Stat(file)
		if err == nil && statErr == nil && string(cur) == f.Content && fi.Mode().Perm() == perm {
			continue
		}
		if err := fsx.WriteAtomic(file, []byte(f.Content), perm); err != nil {
			return changed, err
		}
		if statErr == nil && fi.Mode().Perm() != perm { // WriteAtomic keeps old perms
			if err := os.Chmod(file, perm); err != nil {
				return changed, err
			}
		}
		changed = append(changed, f.Rel)
	}
	// Remove strays: managed names (render.Managed) that aren't wanted, and
	// gids temp files. Anything else in these directories isn't gids' to delete.
	for _, sub := range []string{"", "profiles", "hooks"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return changed, err
		}
		for _, ent := range entries {
			rel := path.Join(sub, ent.Name())
			temp := fsx.IsTemp(ent.Name())
			if ent.IsDir() || want[rel] || !(temp || render.Managed(rel)) {
				continue
			}
			if err := fsx.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
				return changed, err
			}
			if !temp {
				changed = append(changed, rel)
			}
		}
		if sub != "" && len(files) == 0 {
			os.Remove(filepath.Join(dir, sub)) // only succeeds if empty
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// Mutate is the only way commands change config.yaml: load (right before the
// change, never across a prompt), apply fn, validate+save, then heal.
func (e *Env) Mutate(fn func(cfg *config.Config) error) (*config.Config, error) {
	cfg, err := e.Load()
	if err != nil {
		return nil, err
	}
	if err := fn(cfg); err != nil {
		return nil, err
	}
	if err := config.Save(e.Paths.Config, cfg); err != nil {
		return nil, err
	}
	_, _, err = e.Heal(cfg)
	return cfg, err
}

// Install renders the generated files, then (last, so the activation step is
// a single atomic write) adds the block to the global git config.
func (e *Env) Install() (changedGlobal bool, err error) {
	if err := e.git().RequireVersion(MinGit[0], MinGit[1]); err != nil {
		return false, err
	}
	cfg, err := e.Load()
	if err != nil {
		return false, err
	}
	content, exists, err := e.readGlobal()
	if err != nil {
		return false, err
	}
	prev, err := e.PrevHooksPath()
	if err != nil {
		return false, err
	}
	if _, err := e.sync(render.Render(cfg, render.Input{Home: e.Paths.Home, Exe: e.Exe, PrevHooksPath: prev})); err != nil {
		return false, err
	}
	next, err := render.WithBlock(content, render.BlockBody(e.display(e.Paths.Gitconfig), e.display(e.Paths.HooksDir)), exists)
	if err != nil {
		return false, err
	}
	if next == content {
		return false, nil
	}
	return true, fsx.WriteAtomic(e.Paths.GlobalGitconfig, []byte(next), 0o644)
}

// Uninstall removes the block first (deactivating gids in one atomic write),
// then the generated files. config.yaml is kept.
func (e *Env) Uninstall() (changedGlobal bool, err error) {
	content, exists, err := e.readGlobal()
	if err != nil {
		return false, err
	}
	if exists {
		next, remove, err := render.WithoutBlock(content)
		if err != nil {
			return false, err
		}
		switch {
		case remove:
			target, err := fsx.Target(e.Paths.GlobalGitconfig)
			if err != nil {
				return false, err
			}
			if err := fsx.Remove(target); err != nil {
				return false, err
			}
			changedGlobal = true
		case next != content:
			if err := fsx.WriteAtomic(e.Paths.GlobalGitconfig, []byte(next), 0o644); err != nil {
				return false, err
			}
			changedGlobal = true
		}
	}
	_, err = e.sync(nil)
	return changedGlobal, err
}
