// Package invariant checks SPEC §11's invariants against the real files. It's
// used by tests (integration, interruption, random sequences); production
// code never imports it.
package invariant

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gids/internal/app"
	"gids/internal/config"
	"gids/internal/fsx"
	"gids/internal/render"
)

// Check returns every violated invariant (nil if all hold). exe is the gids
// binary that hooks should point to.
func Check(env *app.Env) error {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	p := env.Paths

	// I1: config.yaml loads and validates (Load validates).
	cfg, err := config.Load(p.Config)
	if err != nil {
		fail("I1: %v", err)
		return errors.Join(errs...)
	}

	// I3: zero or one well-formed block.
	global, _ := os.ReadFile(p.GlobalGitconfig)
	installed, err := render.HasBlock(string(global))
	if err != nil {
		fail("I3: %v", err)
	}
	if installed && !strings.Contains(string(global), render.BlockBody(env.Display(p.Gitconfig), env.Display(p.HooksDir))) {
		fail("I3: block body is stale")
	}

	// I2 + I4: generated files are exactly render(config) iff installed.
	got := map[string]string{}
	modes := map[string]fs.FileMode{}
	for _, sub := range []string{"", "profiles", "hooks"} {
		entries, err := os.ReadDir(filepath.Join(p.Dir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			rel := path.Join(sub, e.Name())
			if e.IsDir() || !(fsx.IsTemp(e.Name()) || render.Managed(rel)) {
				continue // not gids' file (render.Managed): gids never touches it
			}
			if fsx.IsTemp(e.Name()) {
				fail("I2: temp file left behind: %s", rel)
				continue
			}
			b, _ := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(rel)))
			fi, _ := e.Info()
			got[rel], modes[rel] = string(b), fi.Mode().Perm()
		}
	}
	want := map[string]render.File{}
	if installed {
		prev, err := env.PrevHooksPath()
		if err != nil {
			fail("I4: %v", err)
		}
		for _, f := range render.Render(cfg, render.Input{Home: p.Home, Exe: env.Exe, PrevHooksPath: prev}) {
			want[f.Rel] = f
		}
	}
	for rel, f := range want {
		c, ok := got[rel]
		switch {
		case !ok:
			fail("I2: missing generated file %s", rel)
		case c != f.Content:
			fail("I2: %s differs from render(config.yaml)", rel)
		case f.Exec && modes[rel]&0o111 == 0:
			fail("I4: hook %s isn't executable", rel)
		}
	}
	var extra []string
	for rel := range got {
		if _, ok := want[rel]; !ok {
			extra = append(extra, rel)
		}
	}
	sort.Strings(extra)
	for _, rel := range extra {
		if installed {
			fail("I2: stray generated file %s", rel)
		} else {
			fail("I4: generated file %s exists while not installed", rel)
		}
	}
	return errors.Join(errs...)
}
