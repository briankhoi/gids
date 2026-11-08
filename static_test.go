package main

// Static checks (SPEC §11, NOTES "Verification project" item 5): rules the
// crash and trace tests rely on, enforced on the source so a future change
// can't quietly break them.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fsMutators are the os functions that change the filesystem.
var fsMutators = map[string]bool{
	"Create": true, "CreateTemp": true, "WriteFile": true, "OpenFile": true,
	"Remove": true, "RemoveAll": true, "Rename": true, "Chmod": true, "Chown": true,
	"Lchown": true, "Chtimes": true, "Mkdir": true, "MkdirAll": true, "MkdirTemp": true,
	"Symlink": true, "Link": true, "Truncate": true,
}

// allowed lists the only non-fsx places that may call an os mutator, and why.
var allowed = map[string]map[string]string{
	"internal/app/lock.go":     {"MkdirAll": "lock dir", "OpenFile": "the lock file itself (never rewritten)"},
	"internal/cmd/hook.go":     {"OpenFile": "opens the terminal (/dev/tty, CONOUT$), not a file"},
	"internal/testhook/on.go":  {"OpenFile": "test builds only: the trace file", "WriteFile": "test builds only: the event gate"},
	"internal/testenv/":        {"*": "test infrastructure (sandbox only; panics outside it)"},
	"internal/sshconfig/":      {}, // read-only
	"internal/fsx/":            {"*": "the one package that changes files: atomic, traced"},
	"internal/testhook/off.go": {},
}

func allowedCall(file, fn string) bool {
	for prefix, fns := range allowed {
		if file == prefix || (strings.HasSuffix(prefix, "/") && strings.HasPrefix(file, prefix)) {
			if _, ok := fns[fn]; ok {
				return true
			}
			if _, ok := fns["*"]; ok {
				return true
			}
		}
	}
	return false
}

// sources parses every non-test Go file in the module.
func sources(t *testing.T) (map[string]*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") && p != "." || d.Name() == "bin" || d.Name() == "dist") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(p)] = f
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, fset
}

// pkgCall reports call's "pkg.Fn" if it's a selector on an identifier.
func pkgCall(call *ast.CallExpr) (pkg, fn string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	return id.Name, sel.Sel.Name
}

// S1: every file change goes through fsx (atomic and traced), except the
// few listed, justified exceptions.
func TestFileChangesGoThroughFsx(t *testing.T) {
	files, fset := sources(t)
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if pkg, fn := pkgCall(call); pkg == "os" && fsMutators[fn] && !allowedCall(name, fn) {
				t.Errorf("%s: os.%s outside fsx; use fsx (atomic + traced) or justify it in static_test.go", fset.Position(call.Pos()), fn)
			}
			return true
		})
	}
}

// S2: inside fsx, every os mutator is preceded in its function by a
// testhook.Point, so the crash and trace tests see every change.
func TestEveryFsxChangeIsAPoint(t *testing.T) {
	files, fset := sources(t)
	for name, f := range files {
		if !strings.HasPrefix(name, "internal/fsx/") {
			continue
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			lastPoint := token.NoPos
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				pkg, fn := pkgCall(call)
				switch {
				case pkg == "testhook" && fn == "Point":
					lastPoint = call.Pos()
				case pkg == "os" && fsMutators[fn] && fn != "Remove" || pkg == "os" && fn == "Remove" && fd.Name.Name != "WriteAtomic":
					if lastPoint == token.NoPos {
						t.Errorf("%s: os.%s in %s has no testhook.Point before it", fset.Position(call.Pos()), fn, fd.Name.Name)
					}
				}
				return true
			})
		}
	}
}

// S3: the only external program gids runs is git (through internal/git,
// which marks each run as a Point). No "rm", "sh -c" or anything else.
func TestOnlyGitIsExecuted(t *testing.T) {
	files, fset := sources(t)
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			pkg, fn := pkgCall(call)
			if pkg != "exec" || (fn != "Command" && fn != "CommandContext") {
				return true
			}
			if strings.HasPrefix(name, "internal/testenv/") {
				return true // test infrastructure runs go, git and the gids binary
			}
			arg := 0
			if fn == "CommandContext" {
				arg = 1
			}
			lit, ok := call.Args[arg].(*ast.BasicLit)
			if name != "internal/git/git.go" || !ok || lit.Value != `"git"` {
				t.Errorf("%s: %s.%s runs something other than git via internal/git", fset.Position(call.Pos()), pkg, fn)
			}
			return true
		})
	}
}

// S4: no recursive delete in any script, Makefile or workflow (the old
// `make clean` ran rm -rf on an overridable variable).
func TestNoRecursiveDeleteInScripts(t *testing.T) {
	rmr := regexp.MustCompile(`\brm\s+(-[a-zA-Z]*[rR]|--recursive)`)
	var paths []string
	for _, g := range []string{"Makefile", "examples/*.sh", ".github/workflows/*.yml", ".goreleaser.yaml"} {
		m, _ := filepath.Glob(g)
		paths = append(paths, m...)
	}
	if len(paths) < 3 {
		t.Fatalf("found too few scripts to check: %v", paths)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if rmr.MatchString(line) {
				t.Errorf("%s:%d: recursive delete: %s", p, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// Negative control: each check must actually fire on a violation, or a
// passing run proves nothing.
func TestStaticChecksCatchViolations(t *testing.T) {
	src := `package x
import ("os"; "os/exec")
func f() { os.RemoveAll("/"); exec.Command("rm", "-rf", "x") }`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var mut, ex int
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			pkg, fn := pkgCall(call)
			if pkg == "os" && fsMutators[fn] && !allowedCall("internal/cmd/x.go", fn) {
				mut++
			}
			if pkg == "exec" && fn == "Command" {
				ex++
			}
		}
		return true
	})
	if mut != 1 || ex != 1 {
		t.Fatalf("checks didn't fire: mutators %d exec %d", mut, ex)
	}
	if !regexp.MustCompile(`\brm\s+(-[a-zA-Z]*[rR]|--recursive)`).MatchString("\trm -rf ./bin $(X)/") {
		t.Fatal("rm -rf not detected")
	}
}
