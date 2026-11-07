# Verifying gids

`make verify` runs everything below. `make test` runs all of it except the slow
exhaustive state search and TLC.

## What is proven, what is tested, and where the line is

| Layer | What it covers | Kind of guarantee |
|---|---|---|
| **TLA+ models** (`tla/`, TLC) | The file protocol under 2 concurrent gids processes, crashes at every step, and user tampering (`GidsFiles`). amend vs a concurrent `git commit` (`Amend`). The commit guard's decisions (`Guard`). `with --once` and empty messages (`CoauthorOnce`) | **Proof, within the stated bounds**: every reachable state of the model is checked |
| **Trace conformance** (`TestTracesFollowTheModeledProtocol`) | The real code's step order is the order the models assume: temp file + rename, everything under the lock, install activates last, uninstall deactivates first, config is saved before sync, amend's single CAS ref update | **Tested** on every command; connects the proofs to the code |
| **Crash at every step** (`TestCrashAtEveryStep`) | The real binary is killed at each of its numbered steps (209 across 10 commands). Then I1/I3 must hold immediately, and everything (I1–I4, I6) after the next gids command | **Exhaustive at step granularity** for these scenarios |
| **Event-gated races** (`TestLock*`) | Two real processes at an exact interleaving point, with and without the lock | **Deterministic**, both sides |
| **Exhaustive state search** (`TestEveryReachableState`) | Every state reachable in a small domain (318 states), and every operation from every state (3,816 transitions). Each is checked against a model and against real git's includeIf | **Complete within the domain** |
| **Static checks** (`static_test.go`) | All file changes go through `fsx` and are numbered steps; only git is executed; no recursive deletes in scripts | **Enforced on the source** |
| Fuzzing, random sequences, pty end-to-end, unit tests | Parsing and quoting, long random walks, the real terminal | **Tested** |

Every invariant in every model has a **negative control**: a `*_neg_*.cfg` that breaks the
model in one realistic way, which TLC *must* catch. `tla/run.sh` fails if one doesn't.
The Go tests do the same: lock tests run with the lock off, the static checks have a
test that plants violations, and each new check was mutation-tested once during
development. A check that can't fail proves nothing.

## What the models abstract away (audit this boundary, VER-12)
- **The filesystem is POSIX.** `rename` is atomic, and `fsync` plus a directory sync make it
  durable. This isn't guaranteed on some network filesystems, which may also lack
  `flock`; gids then runs unlocked, a mode the models cover as `UseLock = FALSE`, whose
  races TLC shows.
- **"The generated files" are one value.** Their multi-file sync is modeled as two steps
  (Mixed, then done). Git can see a half-synced set while a sync runs: a commit made at
  that exact moment may use the old identity. This is accepted, because the window is
  milliseconds and only happens while gids is changing your config.
- **git is trusted:** `update-ref` CAS, `commit-tree`, and git computing the author before
  hooks run. These were verified by experiment (PROGRESS.md) and by the git tests, not proven.
- **The TUI isn't modeled.** Keys and prompts are covered by unit and pty tests (I7).
- **Bounds.** GidsFiles uses 2 processes × 2 commands; Amend uses 3 commits plus 1 concurrent
  commit; Guard uses 4 attempts with 2 interferences; CoauthorOnce uses 3 attempts. The bugs
  these protocols can have show up at 2 processes and short runs. Larger bounds are a
  config change (`tla/*.cfg`) if you want more assurance.

## Known gap, found by TLC and accepted
`with --once`: if git is killed after creating the commit but before the post-commit
hook, the session survives, so the *next* commit gets the co-author trailer too
(`once_neg_known_gap_postcrash`). It's visible and easy to remove. Closing it would take
commit-msg-side bookkeeping, which isn't worth its complexity.

## Running it
```sh
make verify                       # everything (≈7 min)
cd verify/tla && ./run.sh         # just the TLA+ models (≈30 s)
GIDS_VERIFY=1 go test -run TestEveryReachableState ./internal/cmd
```
TLC needs Java 11+ and `tla2tools.jar` v1.7.4 in `verify/tla/` (gitignored):
```sh
cd verify/tla && gh release download v1.7.4 -R tlaplus/tlaplus -p tla2tools.jar
shasum -a 256 tla2tools.jar   # 936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88
```
