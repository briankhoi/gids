// Package testenv isolates tests from the real machine (SPEC §11.1): every
// test that touches HOME, git config or gids' directory runs in a Sandbox.
package testenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// realHome and buildEnv are captured before anything changes HOME. Building
// the binary uses the original environment so Go finds its own module and
// build caches (it touches nothing of gids' or git's).
var (
	realHomes = findRealHomes()
	buildEnv  = os.Environ()
	startDir  = mustGetwd() // the package dir go test started in (inside the real repo)
)

// findRealHomes is every form of the real home: $HOME as the tests started,
// and the account's home from the user database (in case HOME was already
// changed), each also with symlinks resolved.
func findRealHomes() []string {
	var homes []string
	add := func(h string) {
		if h == "" || !filepath.IsAbs(h) {
			return
		}
		homes = append(homes, filepath.Clean(h))
		if r, err := filepath.EvalSymlinks(h); err == nil {
			homes = append(homes, r)
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		add(h)
	}
	if u, err := user.Current(); err == nil {
		add(u.HomeDir)
	}
	return homes
}

func mustGetwd() string {
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return d
}

// leakedGitVars would make git ignore our sandbox (e.g. when tests run from
// inside a git hook or an IDE).
var leakedGitVars = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY",
	"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_AUTHOR_DATE",
	"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE",
	"GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_EDITOR",
	"GIDS_TTY", "GIDS_SKIP_HOOKS", "EMAIL",
	"GIDS_TEST_CRASH_AT", "GIDS_TEST_TRACE", "GIDS_TEST_NO_LOCK",
}

// Main isolates a whole test binary: call it from TestMain. It points HOME
// and friends at a throwaway directory before any test runs, so even a test
// that forgets to make a Sandbox can't reach the real home.
func Main(m *testing.M) {
	dir, err := os.MkdirTemp("", "gids-test-")
	if err != nil {
		panic(err)
	}
	setIsolatedEnv(dir)
	mustBeIsolated()
	// Run from inside the sandbox, so anything that accidentally uses the
	// current directory (git with no Dir, a relative path) can't reach the
	// real repo go test started in.
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	if binDir != "" {
		os.RemoveAll(binDir)
	}
	os.Exit(code)
}

func setIsolatedEnv(home string) {
	for _, v := range leakedGitVars {
		os.Unsetenv(v)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	os.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	os.Setenv("TERM", "dumb")
	os.Setenv("NO_COLOR", "1")
	// A hook run without scripted answers must never prompt on the real
	// terminal: point GIDS_TTY at a file that doesn't exist (non-interactive).
	os.Setenv("GIDS_TTY", filepath.Join(home, ".no-tty"))
}

// mustBeIsolated panics if the environment could reach the real home.
func mustBeIsolated() {
	home, _ := os.UserHomeDir()
	for _, v := range []string{home, os.Getenv("XDG_CONFIG_HOME"), os.Getenv("GIT_CONFIG_GLOBAL")} {
		if v == "" || !filepath.IsAbs(v) {
			panic(fmt.Sprintf("testenv: refusing to run: %q is empty or relative", v))
		}
		forms := []string{filepath.Clean(v)}
		if r, err := filepath.EvalSymlinks(v); err == nil {
			forms = append(forms, r)
		}
		for _, f := range forms {
			for _, real := range realHomes {
				if f == real || strings.HasPrefix(f, real+string(filepath.Separator)) {
					panic(fmt.Sprintf("testenv: refusing to run: %q is inside the real home %q", v, real))
				}
			}
		}
	}
	if os.Getenv("GIT_CONFIG_NOSYSTEM") != "1" {
		panic("testenv: GIT_CONFIG_NOSYSTEM must be 1")
	}
}

// Sandbox is one isolated machine: its own HOME, gids dir and global config.
type Sandbox struct {
	T    testing.TB
	Home string
}

// New makes a fresh sandbox and points the process environment at it for the
// duration of the test (so in-process code sees it too).
func New(t testing.TB) *Sandbox {
	t.Helper()
	home := t.TempDir()
	// Resolve symlinks (macOS /var → /private/var) so paths git prints match.
	if r, err := filepath.EvalSymlinks(home); err == nil {
		home = r
	}
	for _, v := range leakedGitVars {
		if _, ok := os.LookupEnv(v); ok {
			t.Setenv(v, "")
			os.Unsetenv(v)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GIDS_TTY", filepath.Join(home, ".no-tty"))
	mustBeIsolated()
	return &Sandbox{T: t, Home: home}
}

// Path joins elements under the sandbox home.
func (s *Sandbox) Path(elem ...string) string {
	return filepath.Join(append([]string{s.Home}, elem...)...)
}

// GidsDir is the sandbox's gids directory.
func (s *Sandbox) GidsDir() string { return s.Path(".config", "gids") }

// Write creates a file (and parents) under the sandbox home.
func (s *Sandbox) Write(rel, content string) string {
	s.T.Helper()
	p := s.Path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.T.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		s.T.Fatal(err)
	}
	return p
}

// Read returns a file's contents ("" if missing).
func (s *Sandbox) Read(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// Git runs git in dir and fails the test on error.
func (s *Sandbox) Git(dir string, args ...string) string {
	s.T.Helper()
	out, err := s.TryGit(dir, args...)
	if err != nil {
		s.T.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// TryGit runs git in dir and returns combined output and error.
func (s *Sandbox) TryGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Repo creates a git repository at rel (with one commit if commit is true).
func (s *Sandbox) Repo(rel string, commit bool) string {
	s.T.Helper()
	dir := s.Path(rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.T.Fatal(err)
	}
	s.Git(dir, "init", "-q", "-b", "main")
	if commit {
		s.Commit(dir, "initial", "Seed <seed@example.com>")
	}
	return dir
}

// Commit makes a commit with an explicit identity, bypassing all hooks.
func (s *Sandbox) Commit(dir, msg, ident string) string {
	s.T.Helper()
	f := filepath.Join(dir, "f.txt")
	old, _ := os.ReadFile(f)
	if err := os.WriteFile(f, append(old, []byte(msg+"\n")...), 0o644); err != nil {
		s.T.Fatal(err)
	}
	s.Git(dir, "add", "f.txt")
	name, email, _ := strings.Cut(strings.TrimSuffix(ident, ">"), " <")
	s.Git(dir, "-c", "core.hooksPath=/dev/null", "-c", "user.name="+name, "-c", "user.email="+email,
		"commit", "-q", "--no-verify", "-m", msg)
	return s.Git(dir, "rev-parse", "HEAD")
}

var (
	buildOnce sync.Once
	binPath   string
	binDir    string // removed by Main
	buildErr  error
)

// Binary builds the gids binary once per test process.
func Binary(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gids-bin-")
		if err != nil {
			buildErr = err
			return
		}
		binDir, binPath = dir, filepath.Join(dir, "gids")
		root, err := moduleRoot()
		if err != nil {
			buildErr = err
			return
		}
		// -tags gidstest compiles in internal/testhook (crash points, traces);
		// with its GIDS_TEST_* variables unset it behaves like a release build.
		cmd := exec.Command("go", "build", "-tags", "gidstest", "-o", binPath, ".")
		cmd.Dir = root
		cmd.Env = buildEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binPath
}

func moduleRoot() (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir, cmd.Env = startDir, buildEnv
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return filepath.Dir(strings.TrimSpace(string(out))), nil
}

// Timeout bounds every command a test runs. Nothing gids does takes more
// than seconds; a command still running after this is a hang (e.g. a hook
// loop), which fails the test instead of stalling the suite.
const Timeout = 2 * time.Minute

// run executes name with args in dir, in its own process group that is
// killed entirely on timeout, and returns the exit code.
func (s *Sandbox) run(stdout, stderr io.Writer, stdin io.Reader, dir string, env []string, name string, args ...string) int {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.WaitDelay = 5 * time.Second
	ownGroup(cmd)
	err := cmd.Run()
	if ctx.Err() != nil {
		s.T.Fatalf("%s %s: still running after %v (a hang?); killed its process group", name, strings.Join(args, " "), Timeout)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		s.T.Fatal(err)
	}
	return 0
}

// Proc is a gids process started with Start.
type Proc struct {
	s         *Sandbox
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	out, errb bytes.Buffer
	done      *Result
}

// Start runs gids in the background (own process group, killed after
// Timeout). Wait for it with Wait.
func (s *Sandbox) Start(dir string, env []string, args ...string) *Proc {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	p := &Proc{s: s, cancel: cancel}
	p.cmd = exec.CommandContext(ctx, Binary(s.T), args...)
	p.cmd.Dir, p.cmd.Env = dir, append(os.Environ(), env...)
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.errb
	p.cmd.WaitDelay = 5 * time.Second
	ownGroup(p.cmd)
	if err := p.cmd.Start(); err != nil {
		cancel()
		s.T.Fatal(err)
	}
	s.T.Cleanup(func() { // never leave it running
		if p.done == nil {
			cancel()
			p.cmd.Wait()
		}
	})
	return p
}

// Wait waits for the process and returns its result (Code -1 if killed).
// Calling it again returns the same result.
func (p *Proc) Wait() Result {
	p.s.T.Helper()
	if p.done != nil {
		return *p.done
	}
	defer p.cancel()
	err := p.cmd.Wait()
	var ee *exec.ExitError
	code := 0
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
	case err != nil:
		p.s.T.Fatal(err)
	}
	p.done = &Result{Out: p.out.String(), Err: p.errb.String(), Code: code}
	return *p.done
}

// WaitFile polls until path exists (an event gate's ".reached" marker),
// failing the test after Timeout.
func (s *Sandbox) WaitFile(path string) {
	s.T.Helper()
	deadline := time.Now().Add(Timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			s.T.Fatalf("%s never appeared", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Result of running the gids binary.
type Result struct {
	Out, Err string
	Code     int
}

// Gids runs the built binary in dir with stdin and extra env vars.
func (s *Sandbox) Gids(dir, stdin string, env []string, args ...string) Result {
	s.T.Helper()
	var out, errb bytes.Buffer
	code := s.run(&out, &errb, strings.NewReader(stdin), dir, env, Binary(s.T), args...)
	return Result{Out: out.String(), Err: errb.String(), Code: code}
}

// MustGids is Gids that fails the test on a non-zero exit.
func (s *Sandbox) MustGids(dir string, args ...string) string {
	s.T.Helper()
	r := s.Gids(dir, "", nil, args...)
	if r.Code != 0 {
		s.T.Fatalf("gids %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), r.Code, r.Out, r.Err)
	}
	return r.Out
}

// GitCommit runs a real `git commit` (hooks enabled) with scripted guard
// answers in ttyAnswers (via GIDS_TTY). It returns combined output and the
// exit code.
func (s *Sandbox) GitCommit(dir, ttyAnswers string, args ...string) (string, int) {
	s.T.Helper()
	// Make sure the hook finds the test binary on PATH-independent hooks.
	answers := filepath.Join(s.T.TempDir(), "tty")
	if err := os.WriteFile(answers, []byte(ttyAnswers), 0o644); err != nil {
		s.T.Fatal(err)
	}
	f := filepath.Join(dir, "f.txt")
	old, _ := os.ReadFile(f)
	os.WriteFile(f, append(old, []byte("change\n")...), 0o644)
	s.Git(dir, "add", "f.txt")
	var out bytes.Buffer
	code := s.run(&out, &out, nil, dir, []string{"GIDS_TTY=" + answers}, "git", append([]string{"commit", "-q"}, args...)...)
	return out.String(), code
}

// Author is HEAD's "Name <email>" in dir.
func (s *Sandbox) Author(dir string) string {
	s.T.Helper()
	return s.Git(dir, "log", "-1", "--format=%an <%ae>")
}
