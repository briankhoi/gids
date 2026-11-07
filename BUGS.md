# Known bugs (v1 code, audited 2026-11-07)

## Status on branch `v2-includeif` (2026-11-07)
| Bug | Status | Proven by |
|---|---|---|
| B1 prompt EOF loop | **Fixed**: EOF ends every prompt (`ErrSkipped`) | `TestFallbackInputNeverLoopsForever`, `TestGuardEOFAndBadAnswersNeverHang` |
| B2 hook stdin | **Fixed**: guard uses `/dev/tty` | `TestE2EGuardOnRealTerminal` (real pty) |
| B3 identity in pre-commit | **Fixed by design**: fix config, then stop and re-run | `TestUnmappedWrongIdentityPickProfileThenRerun`, `TestMismatchFromLocalOverrideIsFixedThenRerun` |
| B4 demo stdin | **Fixed**: new `examples/demo.sh` uses scripted `GIDS_TTY` | ran end-to-end |
| B5 subdirectories | **Gone**: git `gitdir:` matching | `TestRuleAppliesEverywhereBeneathIt` |
| B6 hooksPath clobbering | **Fixed**: shims chain; block-scoped hooksPath restores exactly | `TestRepoHooksStillRun`, `TestPreviousGlobalHooksPathIsChained` |
| B7 IdentitiesOnly | **Fixed** | `TestRenderProfileReadsBackWithGit` |
| B8 SSH signing format | **Fixed**: `gpg.format = ssh` | `TestRenderProfileReadsBackWithGit`, `TestIsSSHSigningKey` |
| B9 config vs real author | **Fixed**: uses `GIT_AUTHOR_*` | `TestMismatchFromAuthorFlagOnlyWarns` |
| B10 local hooksPath bypass | **Limitation, now reported** by `status` | `TestStatusWarnings` |
| B11–B13 | **Gone** (no per-repo writes, no shell hook) | — |
| B14 duplicate names | **Fixed**: case-insensitive uniqueness everywhere | `TestProfileMutations`, `TestProfileCommands` |
| B15 rule remove | **Fixed**: same normalization for add and remove | `TestRuleCommands`, `FuzzNormalizeRule` |
| B16 false "removed" | **Fixed**: damaged blocks are refused | `TestMalformedBlocksAreRefused` |
| B17 misleading status | **Fixed** | `TestStatusWarnings` |
| B18 config dir | **Fixed**: `~/.config/gids` | `TestResolvePaths` |
| S1–S3 spec bugs | **Fixed in SPEC v2** (amend uses plumbing, not rebase) | `TestAmendLastNIncludingRootAndMerges` |

### New bugs found and fixed while building v2
- **B19 · H · Release binaries always reported version "dev":** `.goreleaser.yaml` set
  `-X github.com/briankhoi/gids/...`, but the module is `gids`. Verified with both flags;
  fixed.
- **B20 · M · Symlinked paths stored rules as absolute paths** (found by the demo): git
  reports resolved paths (macOS `/var` → `/private/var`, or `~/code → /Volumes/…`).
  Normalization now treats the resolved home as `~`. Test:
  `TestSymlinkedHomeKeepsTildeRules`.
- **B21 · H (spec) · A co-author trailer from `prepare-commit-msg` defeats git's
  empty-message abort.** Verified in a sandbox; v2 uses `commit-msg` (PROGRESS D1).
  Test: `TestWithAddsTrailersAndRespectsEmptyMessages`.
- **B22 · M · A gids panic or startup error inside a hook would have blocked commits**
  (found in code review). Hooks now fail open. Test: `TestHookFailsOpenWithoutHome`.
- **B23 · L · `go mod tidy` silently downgraded the pinned `creack/pty` to v1.1.9** (MVS
  picked another module's minimum). Re-pinned v1.1.24; CI's module scan would catch a
  vulnerable downgrade.

Severity: **C** critical · **H** high · **M** medium · **L** low · **–** cleanup.
"v2" says what happens to the bug under `SPEC.md` v2. *Gone* means the code goes away.

## Critical

### B1 · C · Prompts loop forever when input runs out
`internal/cmd/prompt.go:42`. `prompt()` returns `("", nil)` on `io.EOF` even when it
read nothing, so `promptRequired` loops forever and `confirmPrompt` silently returns its
default.
- **Repro (verified):** unmapped repo, no matching profile, `git commit` → the guard printed
  `This field is required.` 1.9M times in 5 s, and the commit never finishes.
- Also: `gids use work < /dev/null` silently saves a rule (EOF counts as "yes").
- Introduced by `2ed16c2 fix: treat EOF-with-data as success`. Only EOF *with data*
  should succeed.
- **v2:** still applies. Fix: return `io.EOF` when `line == ""`.

### B2 · C · The pre-commit guard can't read user input
`internal/cmd/hook.go:17`. Git runs hooks with stdin = `/dev/null`, so every guard prompt
reads EOF (then hits B1). Verified: `[ -t 0 ]` is false inside the hook and `/dev/tty`
is readable.
- **v2:** the guard reads `/dev/tty`, and is non-interactive when that's unavailable (§7).

### B3 · C · Changing identity in pre-commit doesn't affect the commit in progress
`internal/cmd/guard.go` (all `git.Apply` calls). Git settles the author before the
hook runs and exports it as `GIT_AUTHOR_NAME/EMAIL`.
- **Repro (verified):** mapped repo with the wrong identity → guard "fixes" `.git/config` to
  `work@co.com`, but `git log -1` still shows `Global Me <global@x.com>`.
- The v1 spec's "applies it to the current commit" (§3.D/§3.H) can't be done.
- **v2:** fix the config, then abort with "re-run your commit" (§8).

## High

### B4 · H · Demo scripts pipe answers to `git commit`, and those never reach the hook
`examples/demo-phase5.sh:125,152,176`. Because of B2 the piped `y` is ignored, so the
flows only "work" because the prompts default to Y. Line 176 (no profiles → wizard)
should hang through B1. That's inferred, not run.
- **v2:** the demo drives prompts through `GIDS_TTY`.

### B5 · H · Rules don't apply in subdirectories
`internal/config/rules.go:41` (`filepath.Match`) and `internal/cmd/use.go:60`.
`*` doesn't cross `/`, so `~/work/*` matches `~/work/foo` but not `~/work/foo/src`. Rules
saved by `use` and the guard are exact paths, so `cd src` stops switching, and `use`
run from a subdir saves a rule for that subdir.
- **v2:** *gone*. Git's `gitdir:` matching covers every depth, and the rule is the repo root.

### B6 · H · Global `core.hooksPath` silently disables every repo's own hooks
`internal/cmd/hook.go:41,49`. Setting `core.hooksPath` globally replaces `.git/hooks`
everywhere (husky, pre-commit, lint hooks stop running). It also overwrites any existing
global `core.hooksPath`, and `uninstall` unsets it even if gids didn't set it.
- **v2:** the shims chain to the previous hooksPath or the repo's own hooks; save and restore it (§7).

### B7 · H · `core.sshCommand` is missing `IdentitiesOnly=yes`
`internal/git/apply.go:84`. Without it, ssh-agent may offer another account's key first,
and GitHub (and others) will authenticate you as *that* account. This is the classic
multi-account failure. Not reproduced here, but it's well known.
- **v2:** `ssh -i '<key>' -o IdentitiesOnly=yes` (§3.3).

## Medium

### B8 · M · SSH signing keys are used as GPG key IDs
`internal/git/apply.go:36`. `user.signingKey` is set to a path such as `~/.ssh/id.pub`
without `gpg.format = ssh`, so signing calls `gpg` with a file path and fails.
- **v2:** set `gpg.format = ssh` when the key is a path (§3.3).

### B9 · M · The guard checks config identity, not the commit's real author
`internal/cmd/guard.go:53`. `git commit --author=…` or `GIT_AUTHOR_*` env can make the
real author differ from `user.email`, which gives false warnings or missed ones.
- **v2:** read `GIT_AUTHOR_NAME/EMAIL` from the hook's environment (§8).

### B10 · M · Repos with a local `core.hooksPath` bypass the guard
Repos that set their own hooksPath (husky) never run gids' hooks. There's no warning.
- **v2:** still applies (a git limitation). `status` warns (§6.1).

## Low

### B11 · L · `Apply` deletes local settings the user set themselves
`internal/git/apply.go:45` (`setOrUnset`). Applying a profile with no SSH key unsets
the repo's own `core.sshCommand` / `credential.username` / `user.signingKey`.
- **v2:** *gone* (gids no longer writes `.git/config`).

### B12 · L · The bash hook runs on every prompt, not on `cd`
`internal/cmd/hook.go:66`. `PROMPT_COMMAND` → `gids check` → about 6 git processes
per prompt.
- **v2:** *gone* (no shell hook).

### B13 · L · Wrong PowerShell profile path on Windows
`internal/cmd/hook.go:96`. `~/.config/powershell/…` is the Linux pwsh location. Windows uses
`~\Documents\PowerShell\Microsoft.PowerShell_profile.ps1`.
- **v2:** *gone*.

### B14 · L · Guard quick-create allows duplicate profile names
`internal/cmd/guard.go:127`. The no-matching-profile path appends a profile without
checking whether the name already exists.
- **v2:** use a shared create-profile helper that validates uniqueness.

### B15 · L · `rule remove <path>` needs the exact stored text
`internal/cmd/rule.go:121`. `remove ~/work` won't remove `~/work/*`, and relative paths
aren't resolved.
- **v2:** `add` and `remove` normalize the path the same way (§5).

### B16 · L · `uninstall` reports "removed" when it removed nothing
`internal/cmd/hook.go:128,295`. If the end marker is missing, `removeHook` returns the
content unchanged, but `uninstall` still prints `Shell hook removed`. The comment on
`hookInstalled` claims this case is handled.
- **v2:** the same marker-block code is reused for `~/.gitconfig`. Return a "changed" bool and report honestly.

### B17 · L · `status` is misleading
`internal/cmd/status.go:46,72`. It reads only `--local` identity, so an unmapped repo
using the global identity shows `(not set)`. It shows `Source: Rule` even when the
identity doesn't match the rule.
- **v2:** effective identity plus origin (§6.1).

### B18 · L · Config dir doesn't match the spec on macOS
`internal/config/config.go:13`. `os.UserConfigDir()` gives
`~/Library/Application Support/gids`, while the v1 spec says `~/.config/gids`.
- **v2:** use `~/.config/gids` (decided, §10.3).

## Spec bugs (v1 text, not code)
- **S1:** §3.G `git rebase … HEAD~N` fails when N equals the commit count (needs `--root`) and
  flattens merges (needs `--rebase-merges`).
- **S2:** §3.G The `prepare-commit-msg` hook would add session co-authors during
  `gids amend`, which breaks its "never touches co-authors" guarantee.
- **S3:** §3.D says `hook install` writes `prepare-commit-msg`, but that's Phase 6.

## Cleanup
- **–** The logger is created and put in the context (`internal/cmd/root.go:37`) but never read.
- **–** `gofmt -l` flags 6 files, and CI doesn't check formatting.
- **–** ~~Scratch files `test.txt`, `test2.txt`, `test3.md`~~ trashed 2026-11-07.
- **–** ~~14 merged local branches~~ deleted 2026-11-07 (all were merged; already gone on `origin`).
