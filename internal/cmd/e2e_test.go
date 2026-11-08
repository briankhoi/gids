//go:build !windows

package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"gids/internal/testenv"
)

// term is a process running on a real pseudo-terminal, driven like a person.
type term struct {
	t    *testing.T
	pty  *os.File
	tty  *os.File
	cmd  *exec.Cmd
	mu   sync.Mutex
	out  bytes.Buffer
	done chan struct{}
}

// startTerm runs name args in dir on a fresh pty (as the session's
// controlling terminal, so /dev/tty inside git hooks is this pty).
func startTerm(t *testing.T, dir string, env []string, name string, args ...string) *term {
	t.Helper()
	ptm, tty, err := pty.Open()
	if err != nil {
		t.Skip("no pty:", err)
	}
	pty.Setsize(ptm, &pty.Winsize{Rows: 24, Cols: 80})
	// Run through sh so that after the program exits, `stty -a` reports the
	// terminal's state from inside the same session (SPEC §7.2: the terminal
	// is always restored). The program's exit code is preserved.
	wrapper := `"$0" "$@"; code=$?; printf '\n<<STTY>>'; stty -a; exit $code`
	cmd := exec.Command("sh", append([]string{"-c", wrapper, name}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append([]string{"TERM=xterm-256color", "GIDS_TTY="}, env...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	tm := &term{t: t, pty: ptm, tty: tty, cmd: cmd, done: make(chan struct{})}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptm.Read(buf)
			tm.mu.Lock()
			tm.out.Write(buf[:n])
			tm.mu.Unlock()
			if err != nil {
				close(tm.done)
				return
			}
		}
	}()
	t.Cleanup(func() { cmd.Process.Kill(); ptm.Close(); tty.Close() })
	return tm
}

func (tm *term) screen() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.out.String()
}

// waitFor blocks until the output contains s.
func (tm *term) waitFor(s string) {
	tm.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(tm.screen(), s) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	tm.t.Fatalf("timed out waiting for %q; screen:\n%q", s, tm.screen())
}

func (tm *term) send(s string) {
	tm.t.Helper()
	time.Sleep(60 * time.Millisecond) // let the program read the previous key
	if _, err := tm.pty.WriteString(s); err != nil {
		tm.t.Fatal(err)
	}
}

// wait returns the exit code.
func (tm *term) wait() int {
	tm.t.Helper()
	errc := make(chan error, 1)
	go func() { errc <- tm.cmd.Wait() }()
	select {
	case err := <-errc:
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		if err != nil {
			tm.t.Fatal(err)
		}
		return 0
	case <-time.After(15 * time.Second):
		tm.t.Fatalf("process didn't exit; screen:\n%q", tm.screen())
		return -1
	}
}

// cooked reports whether the terminal was back in normal (canonical, echo)
// mode after the program exited, per `stty -a` run right after it.
func (tm *term) cooked() bool {
	tm.t.Helper()
	tm.waitFor("<<STTY>>")
	time.Sleep(100 * time.Millisecond)
	_, st, _ := strings.Cut(tm.screen(), "<<STTY>>")
	fields := strings.Fields(st)
	has := func(f string) bool {
		return slices.Contains(fields, f)
	}
	return has("icanon") && has("echo") && !has("-icanon") && !has("-echo")
}

const (
	keyDown  = "\x1b[B"
	keyUp    = "\x1b[A"
	keyEnter = "\r"
	keyEsc   = "\x1b"
	ctrlC    = "\x03"
	ctrlD    = "\x04"
	space    = " "
)

func TestE2EUsePickerWithArrowsAndFilter(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/x", true)
	tm := startTerm(t, repo, nil, testenv.Binary(t), "use")
	tm.waitFor("Use which profile here?")
	tm.waitFor("Personal")
	tm.send(keyDown)
	tm.send(keyEnter)
	want(t, tm.wait() == 0, "exit; screen:\n%q", tm.screen())
	want(t, w.cfg().Rules["~/projects/x/"] == "Personal", "arrows picked %v", w.cfg().Rules)
	want(t, tm.cooked(), "terminal left in raw mode")

	repo2 := w.Repo("projects/y", true)
	tm = startTerm(t, repo2, nil, testenv.Binary(t), "use")
	tm.waitFor("Personal")
	tm.send("wor")
	tm.send(keyEnter)
	want(t, tm.wait() == 0 && w.cfg().Rules["~/projects/y/"] == "Work", "filter picked %v", w.cfg().Rules)
	w.check()
}

func TestE2EPickerCtrlCAndEscAndCtrlD(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/x", true)
	for key, code := range map[string]int{ctrlC: 130, keyEsc: 1, ctrlD: 1} {
		tm := startTerm(t, repo, nil, testenv.Binary(t), "use")
		tm.waitFor("Personal")
		tm.send(key)
		got := tm.wait()
		want(t, got == code, "key %q: exit %d, want %d; screen:\n%q", key, got, code, tm.screen())
		want(t, tm.cooked(), "key %q left the terminal raw", key)
	}
	want(t, len(w.cfg().Rules) == 0, "cancelled picker saved a rule: %v", w.cfg().Rules)
	w.check()
}

func TestE2EResizeMidPicker(t *testing.T) {
	w := newWorld(t, true)
	for i := range 15 {
		w.MustGids(w.Home, "profile", "add", "--name", "p"+string(rune('a'+i)), "--git-name", "P", "--email", "p@x"+string(rune('a'+i)))
	}
	repo := w.Repo("projects/x", true)
	tm := startTerm(t, repo, nil, testenv.Binary(t), "use")
	tm.waitFor("of 17")
	pty.Setsize(tm.pty, &pty.Winsize{Rows: 6, Cols: 30})
	tm.cmd.Process.Signal(syscall.SIGWINCH)
	tm.send(keyUp)
	tm.send(keyDown + keyDown)
	tm.send(keyEnter)
	want(t, tm.wait() == 0, "exit; screen:\n%q", tm.screen())
	want(t, w.cfg().Rules["~/projects/x/"] == "pa", "picked %v", w.cfg().Rules)
}

// The real thing: git commit → pre-commit hook → gids opens /dev/tty (this pty).
func TestE2EGuardOnRealTerminal(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/bot", true)
	os.WriteFile(filepath.Join(repo, "f.txt"), []byte("x\n"), 0o644)
	w.Git(repo, "add", "f.txt")
	head := w.Git(repo, "rev-parse", "HEAD")

	// Ctrl-C at "Is this right?" aborts the commit and changes nothing.
	tm := startTerm(t, repo, nil, "git", "commit", "-m", "msg")
	tm.waitFor("Is this right?")
	tm.send(ctrlC)
	want(t, tm.wait() != 0, "commit should be aborted")
	want(t, w.Git(repo, "rev-parse", "HEAD") == head && len(w.cfg().Rules) == 0, "Ctrl-C changed state")
	want(t, tm.cooked(), "terminal left raw after Ctrl-C in a hook")

	// Ctrl-D skips: the commit goes through unchanged.
	tm = startTerm(t, repo, nil, "git", "commit", "-m", "msg")
	tm.waitFor("Is this right?")
	tm.send(ctrlD)
	want(t, tm.wait() == 0 && w.Author(repo) == globalID, "Ctrl-D: author %q", w.Author(repo))

	// n → picker → filter "pers" → Enter: rule saved, commit stopped for re-run.
	os.WriteFile(filepath.Join(repo, "f.txt"), []byte("y\n"), 0o644)
	w.Git(repo, "add", "f.txt")
	head = w.Git(repo, "rev-parse", "HEAD")
	tm = startTerm(t, repo, nil, "git", "commit", "-m", "msg")
	tm.waitFor("Is this right?")
	tm.send("n")
	tm.waitFor("Commit as which profile?")
	tm.send("pers")
	tm.send(keyEnter)
	tm.waitFor("Re-run your commit")
	want(t, tm.wait() != 0 && w.Git(repo, "rev-parse", "HEAD") == head, "should stop for re-run")
	tm = startTerm(t, repo, nil, "git", "commit", "-m", "msg")
	want(t, tm.wait() == 0 && w.Author(repo) == meID, "re-run author %q\n%q", w.Author(repo), tm.screen())
	w.check()
}

func TestE2EWithChecklistAndProfileForm(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/x", true)
	tm := startTerm(t, repo, nil, testenv.Binary(t), "with")
	tm.waitFor("Co-authors for this repo")
	tm.send(space)           // Work
	tm.send(keyDown + space) // Personal
	tm.send(keyEnter)
	want(t, tm.wait() == 0, "exit; %q", tm.screen())
	got := w.MustGids(repo, "with")
	want(t, strings.Contains(got, workID) && strings.Contains(got, meID), "session %q", got)

	tm = startTerm(t, w.Home, nil, testenv.Binary(t), "profile", "add")
	tm.waitFor("New profile")
	tm.send("School\t")
	tm.send("Brian Nguyen\t")
	tm.send("bqn@andrew.cmu.edu")
	tm.send(keyEnter + keyEnter + keyEnter + keyEnter) // through the optional fields, then save
	want(t, tm.wait() == 0, "exit; %q", tm.screen())
	p := w.cfg().Profile("School")
	want(t, p != nil && p.GitEmail == "bqn@andrew.cmu.edu", "form saved %+v", p)
	want(t, tm.cooked(), "terminal left raw")
}

func TestE2ENoColorHonored(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/x", true)
	tm := startTerm(t, repo, []string{"NO_COLOR=1"}, testenv.Binary(t), "use", "Work")
	want(t, tm.wait() == 0, "%q", tm.screen())
	for _, sgr := range []string{"\x1b[32m", "\x1b[33m", "\x1b[31m", "\x1b[92m"} {
		want(t, !strings.Contains(tm.screen(), sgr), "color %q despite NO_COLOR: %q", sgr, tm.screen())
	}
	tm = startTerm(t, repo, []string{"NO_COLOR="}, testenv.Binary(t), "use", "Work")
	tm.wait()
	want(t, strings.Contains(tm.screen(), "✓"), "%q", tm.screen())
}
