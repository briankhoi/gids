package cmd_test

// The cross-process lock (SPEC §7.2), tested with event gates: a test build
// pauses one gids at an exact step while another runs, so each race is
// deterministic. Each test also runs with the lock disabled (a test-build
// switch) as a negative control: it must then see the bug.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gids/internal/testenv"
)

// pauseAt finds, on a throwaway world, the first step of args matching op
// and a path suffix; then makes the real world (last, so the process
// environment points at it) and starts gids there paused (event gate) at
// that step. It returns the world, the paused process and the gate file
// that releases it.
func pauseAt(t *testing.T, env []string, op, suffix string, args ...string) (*world, *testenv.Proc, string) {
	t.Helper()
	probe := newWorld(t, true)
	steps, _ := traceEnv(t, probe, env, probe.Home, "", args...) // same env: same step numbering
	k := slices.IndexFunc(steps, func(s testenv.Step) bool { return s.Op == op && strings.HasSuffix(s.Arg, suffix) })
	if k < 0 {
		t.Fatalf("no %s %s step in %v", op, suffix, steps)
	}
	w := newWorld(t, true)
	gate := filepath.Join(t.TempDir(), "gate")
	p := w.Start(w.Home, append([]string{"GIDS_TEST_PAUSE_AT=" + strconv.Itoa(k+1), "GIDS_TEST_PAUSE_GATE=" + gate}, env...), args...)
	w.WaitFile(gate + ".reached")
	return w, p, gate
}

// Process A has loaded config.yaml and is about to save it when process B
// runs a whole `profile add`. Without the lock, A's save drops B's profile
// (a lost update); with it, B waits for A, so both profiles survive.
func TestLockPreventsLostUpdates(t *testing.T) {
	for _, lock := range []bool{true, false} {
		t.Run(fmt.Sprint("lock=", lock), func(t *testing.T) {
			var env []string // testenv clears GIDS_TEST_*, so pass it explicitly
			if !lock {
				env = []string{"GIDS_TEST_NO_LOCK=1"}
			}
			w, a, gate := pauseAt(t, env, "create-temp", "config.yaml", "profile", "add", "--name", "A", "--git-name", "A", "--email", "a@x")
			b := w.Start(w.Home, env, "profile", "add", "--name", "B", "--git-name", "B", "--email", "b@x")
			if !lock {
				if r := b.Wait(); r.Code != 0 { // B finishes while A is paused
					t.Fatalf("B: %+v", r)
				}
			}
			os.WriteFile(gate, nil, 0o600) // release A
			ra, rb := a.Wait(), b.Wait()
			got := w.cfg()
			both := got.Profile("A") != nil && got.Profile("B") != nil
			if lock {
				want(t, ra.Code == 0 && rb.Code == 0 && both, "with the lock both must survive: A %+v B %+v profiles %v", ra, rb, got.Profiles)
				w.check()
			} else {
				// Negative control: proves this test can see the bug at all.
				want(t, !both, "without the lock B's profile should be lost, but got %v", got.Profiles)
			}
		})
	}
}

// Process A is writing config.yaml (its temp file exists) when process B
// runs `gids status`, whose heal deletes leftover temp files. Without the
// lock B deletes A's in-flight temp file and A fails; with it, B waits.
func TestLockProtectsInFlightWrites(t *testing.T) {
	for _, lock := range []bool{true, false} {
		t.Run(fmt.Sprint("lock=", lock), func(t *testing.T) {
			var env []string
			if !lock {
				env = []string{"GIDS_TEST_NO_LOCK=1"}
			}
			w, a, gate := pauseAt(t, env, "rename", "config.yaml", "profile", "add", "--name", "A", "--git-name", "A", "--email", "a@x")
			b := w.Start(w.Home, env, "status")
			if !lock {
				b.Wait()
			}
			os.WriteFile(gate, nil, 0o600)
			ra := a.Wait()
			b.Wait()
			if lock {
				want(t, ra.Code == 0 && w.cfg().Profile("A") != nil, "with the lock A must succeed: %+v", ra)
				w.check()
			} else {
				want(t, ra.Code != 0, "without the lock A's temp file should be deleted under it, but A succeeded")
			}
		})
	}
}
