//go:build gidstest

package testhook

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// Test-build controls (all unset = behave exactly like a normal build):
//
//	GIDS_TEST_CRASH_AT=N  SIGKILL this process at the Nth Point (1-based)
//	GIDS_TEST_TRACE=file  append "op path" for every Point to file
//	GIDS_TEST_NO_LOCK=1   skip the cross-process lock (negative control)
//	GIDS_TEST_PAUSE_AT=N, GIDS_TEST_PAUSE_GATE=f
//	                      at the Nth Point, create f.reached and wait until f
//	                      exists: an event gate that lets a test run another
//	                      process at exactly this interleaving point
var (
	pauseAt, _ = strconv.Atoi(os.Getenv("GIDS_TEST_PAUSE_AT"))
	gate       = os.Getenv("GIDS_TEST_PAUSE_GATE")
	crashAt, _ = strconv.Atoi(os.Getenv("GIDS_TEST_CRASH_AT"))
	trace      = os.Getenv("GIDS_TEST_TRACE")
	noLock     = os.Getenv("GIDS_TEST_NO_LOCK") == "1"
	n          atomic.Int64
)

func Point(op, path string) {
	i := n.Add(1)
	if trace != "" {
		if f, err := os.OpenFile(trace, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintf(f, "%s %s\n", op, path)
			f.Close()
		}
	}
	if pauseAt > 0 && i == int64(pauseAt) && gate != "" {
		os.WriteFile(gate+".reached", nil, 0o600)
		for {
			if _, err := os.Stat(gate); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if crashAt > 0 && i == int64(crashAt) {
		p, _ := os.FindProcess(os.Getpid())
		p.Kill() // SIGKILL: no defers, no flushing, exactly like kill -9
		select {}
	}
}

func NoLock() bool { return noLock }
