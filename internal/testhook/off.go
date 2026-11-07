//go:build !gidstest

package testhook

// Point marks one atomic step (e.g. "rename", path). No-op in normal builds.
func Point(op, path string) {}

// NoLock reports whether the cross-process lock is disabled (only ever in
// test builds, as a negative control). Always false in normal builds.
func NoLock() bool { return false }
