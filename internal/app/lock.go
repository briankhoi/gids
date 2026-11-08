package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gids/internal/testhook"
)

// LockWait is how long a gids command waits for another one to finish
// changing gids' files before giving up. Changes take milliseconds, and no
// lock is ever held across a prompt.
var LockWait = 10 * time.Second

// errLockUnsupported means the filesystem can't lock (e.g. some network
// filesystems); gids then proceeds unlocked rather than refusing to work.
var errLockUnsupported = errors.New("file locking not supported here")

// lock serializes every change to gids' files (config.yaml, the generated
// files, the ~/.gitconfig block) across processes, so two gids commands at
// once can't lose one's update or delete the other's in-flight temp file.
// It's an OS advisory lock on Dir/.lock, released automatically if the
// process dies, so a crash can never leave gids locked.
func (e *Env) lock() (unlock func(), err error) {
	if err := os.MkdirAll(e.Paths.Dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(e.Paths.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if testhook.NoLock() { // test builds only: negative control for the lock
		f.Close()
		return func() {}, nil
	}
	testhook.Point("lock", e.Paths.Dir)
	deadline := time.Now().Add(LockWait)
	for {
		err := tryLock(f)
		if err == nil {
			return func() { testhook.Point("unlock", e.Paths.Dir); f.Close() }, nil // closing releases the lock
		}
		if errors.Is(err, errLockUnsupported) {
			f.Close()
			return func() {}, nil
		}
		if !errors.Is(err, errLocked) || time.Now().After(deadline) {
			f.Close()
			if errors.Is(err, errLocked) {
				return nil, fmt.Errorf("another gids command has been changing %s for over %v; try again", e.display(e.Paths.Dir), LockWait)
			}
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
