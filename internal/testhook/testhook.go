// Package testhook lets tests observe and interrupt gids deterministically
// (SPEC §11): every file and git operation calls Point, which in a test
// build (go build -tags gidstest) can record a trace of operations or kill
// the process at exactly the Nth one. In a normal build Point is an empty
// function the compiler removes, so release binaries contain none of this.
package testhook
