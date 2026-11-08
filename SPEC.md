# GIDS (Git Identity Swap) — Spec v2

> v2 replaces the original (v1) design. The big change: **git does the
> identity switching itself** via `includeIf`; gids manages profiles and rules,
> generates the git config, and guards commits. Breaking changes from v1 are fine —
> gids has one user.
>
> Design decisions and their rationale are recorded in §10.

## 1. Goal

Never commit, push, or sign as the wrong person. Manage several Git identities
(Work, Personal, School) and have the right one apply automatically per directory,
in every tool — terminal, IDE, scripts — with no shell hook.

### Design principles
- **Git is the engine.** Identity selection uses git's native `includeIf "gitdir:…"`.
  gids never reimplements git's path matching and never writes identity into a
  repo's `.git/config`.
- **One source of truth.** `config.yaml` holds profiles and rules. Every git-facing
  file gids owns is *generated* from it and fully rewritten on each change
  (idempotent render, no partial edits).
- **Touch the user's files once.** gids adds one marker-delimited block to
  `~/.gitconfig` and sets `core.hooksPath`. Everything else lives in gids' directory.
- **Never block a commit unexpectedly.** The guard fails open on errors and in
  non-interactive contexts.

## 2. Stack
- Go (version per `go.mod`), Cobra, `gopkg.in/yaml.v3`, `github.com/kevinburke/ssh_config`.
- Bubble Tea v2 (`charm.land/bubbletea/v2`), Bubbles v2 (`charm.land/bubbles/v2`) + Lip Gloss v2
  (`charm.land/lipgloss/v2`) for interactive pickers and styled output (§7.1). Versions in §2.1.
- Git via `os/exec` (no libgit2/go-git).
- Tests: stdlib `testing`, `t.TempDir()`, and real `git` against a temp `HOME`;
  `github.com/creack/pty` for terminal end-to-end tests.
- Requires `git ≥ 2.32` (for `commit --trailer`). `gids install` checks this and refuses older versions.

### 2.1 Dependency and security policy
- **No known vulnerabilities in any dependency version we pin.** This includes the Go
  toolchain, because release binaries bundle the standard library. "Known" means listed in
  the Go vulnerability database (govulncheck, OSV).
- **Before adding or upgrading a module:** run `govulncheck -scan module` with it added.
  Pick the newest release that's at least about 2 weeks old, unless a newer one fixes a
  vulnerability. Pin exact versions in `go.mod`.
- **Pinned as of 2026-11-07** (module scan: 0 findings, including transitive deps):

  | Module | Version |
  |---|---|
  | Go toolchain (`toolchain` directive) | go1.27.3 |
  | `github.com/spf13/cobra` | v1.10.2 |
  | `gopkg.in/yaml.v3` | v3.0.1 |
  | `github.com/kevinburke/ssh_config` | v1.6.0 |
  | `charm.land/bubbletea/v2` | v2.1.0 (v2.1.1 is under 2 weeks old) |
  | `charm.land/bubbles/v2` | v2.2.1 |
  | `charm.land/lipgloss/v2` | v2.0.6 |
  | `github.com/creack/pty` (tests only) | the latest version that passes the scan when added |

- **CI** runs `govulncheck ./...` and `govulncheck -scan module` on every push and on a weekly
  schedule, so newly disclosed vulnerabilities show up without a code change. Any finding
  fails the build.
- **Releases** are built with the pinned toolchain (`go-version-file: go.mod` + `toolchain`).

## 3. Files

| Path | Owner | Contents |
|---|---|---|
| `$GIDS_DIR/config.yaml` | gids (source of truth) | profiles and rules |
| `$GIDS_DIR/gitconfig` | generated | one `includeIf` block per rule |
| `$GIDS_DIR/profiles/<name>.gitconfig` | generated | one profile's git settings |
| `$GIDS_DIR/hooks/{pre-commit,commit-msg,post-commit}` | generated | hook shims (*D1: commit-msg replaces prepare-commit-msg*) |
| `~/.gitconfig` | user; gids adds one block | `[include]` of `$GIDS_DIR/gitconfig` + `core.hooksPath` |

Generated files exist only while gids is installed, i.e. while the block is present (*D5*).

`$GIDS_DIR` = `$XDG_CONFIG_HOME/gids`, else `~/.config/gids` on macOS/Linux;
`%APPDATA%\gids` on Windows. (v1 used `os.UserConfigDir()`, which is
`~/Library/Application Support` on macOS.)

### 3.1 The `~/.gitconfig` block
Appended at the **end** of `~/.gitconfig` (created if missing), between
`# gids:begin` / `# gids:end` markers:

```ini
# gids:begin (managed by `gids install`; remove with `gids uninstall`)
[include]
	path = "~/.config/gids/gitconfig"
[core]
	hooksPath = "~/.config/gids/hooks"
# gids:end
```

*Changed during implementation (D3, D4):*
- **The block also sets `core.hooksPath`.** Because later settings win, removing the block
  automatically restores whatever hooksPath the user had. gids never runs
  `git config --global` edits, which would leave an empty `[core]` header behind and break
  byte-exact uninstall (I8). The previous hooksPath is computed when the files are rendered
  (the last global value that isn't gids', else the system one), not stored.
- **The begin line can carry a tag:** `[+newline]` (gids added a final newline the file
  lacked) or `[created]` (the file didn't exist). Uninstall undoes exactly that, and it
  refuses to touch a file whose markers are damaged.
- **Re-running `install` moves the block to the end** if settings were appended after it.

It must come after any global `[user]` section, because later config wins. gids writes
it directly, not with `git config --global --add include.path`, since that would put it
inside an existing `[include]` section that may come before `[user]`.

### 3.2 Generated `gitconfig`
One block per rule, **sorted least → most specific** (by expanded path length,
ties broken lexically). Git applies includes in order and the last match wins, so the
most specific rule wins:

```ini
[includeIf "gitdir:~/work/"]
	path = profiles/Work.gitconfig
[includeIf "gitdir:~/work/oss-thing/"]
	path = profiles/Personal.gitconfig
```

Relative `path`s resolve against `$GIDS_DIR`. Verified: an `includeIf` inside an
included file works, including from repo subdirectories.

### 3.3 Generated profile file

```ini
[user]
	name = Brian Nguyen
	email = bqn@andrew.cmu.edu
	signingKey = ~/.ssh/id_school.pub
[gpg]
	format = ssh                ; only when signing_key is a path (starts with ~ / . or ends .pub)
[core]
	sshCommand = ssh -i '/Users/brian/.ssh/id_school' -o IdentitiesOnly=yes
[credential]
	username = bqn
[gids]
	profile = School           ; lets gids ask git "which profile applies here?"
```

- Omitted fields are omitted from the file. Nothing is ever "unset".
- `sshCommand`: `~` is expanded at render time, and the path is single-quoted with `'` escaped
  as `'\''`. `IdentitiesOnly=yes` is required, or ssh-agent may offer a different
  account's key first and the host authenticates you as the wrong user.
- gids doesn't turn signing on (`commit.gpgSign`). That stays the user's global choice.
- Values are written with proper git-config quoting, never by string concatenation.

## 4. Profiles

```yaml
profiles:
  - name: School            # [A-Za-z0-9._-]+, unique; becomes the filename
    git_name: Brian Nguyen  # required
    git_email: bqn@andrew.cmu.edu  # required
    username: bqn           # optional → credential.username
    ssh_key: ~/.ssh/id_school      # optional → core.sshCommand
    signing_key: ~/.ssh/id_school.pub  # optional → user.signingKey (+ gpg.format)
```

gids stores no secrets, only names, emails, usernames, and paths.

## 5. Rules

```yaml
rules:
  ~/work/: Work
  ~/school/15-213/: School
```

- A rule is a **directory** that maps to a profile. It covers every repo whose git dir is
  inside it, at any depth. On storage gids resolves relative input to an absolute
  path (shown as `~/…`) and adds a trailing `/`.
- Patterns pass straight to git's `gitdir:` matching, so git glob syntax (`*`, `**`)
  works if the user wants it. gids itself never matches paths.
- The rule for "this repo" is the **parent of `git rev-parse --git-common-dir`**
  (for a normal repo that's the top level), so linked worktrees are covered too.

## 6. Commands

| Command | Behavior |
|---|---|
| `gids install` | Render all files, then write the `~/.gitconfig` block (activation last, as one atomic write). Idempotent. |
| `gids uninstall` | Remove the block (restoring any previous `core.hooksPath`, see §3.1), then the generated files. Leaves `config.yaml` alone. |
| `gids profile add\|list\|edit\|delete` | See §6.2. |
| `gids profile import [--file] [--host]` | SSH config import, §6.3. |
| `gids rule add <dir> <profile>` | Add or replace a rule. |
| `gids rule list` | Table of rules in render order. |
| `gids rule remove <dir>` | Remove a rule (path normalized the same way as `add`). |
| `gids use [<profile>]` | Map **the current repo** (§5) to `<profile>`, i.e. `rule add` for this repo. No arg → profile picker (§7.1). If the repo has local identity overrides (§8), offer to remove them. |
| `gids status` | See §6.1. |
| `gids with …` | Co-authors (§9.1). |
| `gids amend …` | Fix authorship (§9.2). |
| `gids hook-exec <hook> [args]` | Hidden. Entry point for the hook shims. |

Every command that changes `config.yaml` re-renders the generated files (and only
those). A one-off identity that isn't a rule is plain `git config --local user.email …`.
gids has no command for it, and the guard reports it as an override.

### 6.1 `gids status`
```
Identity:   Brian Nguyen <bqn@andrew.cmu.edu>   (from profiles/School.gitconfig)
Profile:    School                               (rule ~/school/)
Co-authors: Alice Smith <alice@example.com>
```
- Identity and origin come from `git config --show-origin --get user.email`.
- Profile is `git config --get gids.profile`, and the rule is the one whose profile matches.
- Warn if the identity is overridden (origin isn't the profile file), if the repo
  sets its own `core.hooksPath` (so the guard can't run here), or if gids isn't installed.

### 6.2 Profile commands
- `profile add`: prompts for profile name, git name, git email (all required), then
  username, SSH key, and signing key (optional, Enter to skip). The name must be valid (§4)
  and unique. If no username, SSH key, or signing key is given, print a note that push/pull
  uses the system default.
- `profile list`: table of NAME, GIT NAME, EMAIL, SSH KEY, plus how many rules use each profile.
- `profile edit <name>`: re-prompts every field with the current value as the default
  (Enter keeps it; `none` clears an optional field). Renaming also updates the rules that
  reference the profile.
- `profile delete <name>`: asks for confirmation (`--force` skips it). Refuses if rules
  reference the profile unless `--force`, which also removes those rules.

### 6.3 `gids profile import [--file <path>] [--host <substr>]`
Creates profiles from `Host` blocks in an SSH config.
- **Choosing the file:** with no `--file`, list the platform's known paths (macOS/Linux:
  `~/.ssh/config`, `/etc/ssh/ssh_config`; Windows: `%USERPROFILE%\.ssh\config`,
  `%ProgramData%\ssh\ssh_config`) with whether each exists, then let the user pick one or
  enter a custom path.
- **Parsing:** `internal/sshconfig`, a thin adapter over `kevinburke/ssh_config`. `Include`
  directives are followed. Wildcard hosts are skipped.
- **Selecting:** list the hosts with their `IdentityFile` and `User`. "Import all? [Y/n]", or
  confirm each one. `--host` keeps only hosts whose pattern contains the substring
  (case-insensitive).
- **For each host:** prompt for git name (after the first host, the previous value is the
  default) and git email (always prompted). Then print a summary table and a
  `gids profile edit` hint.
- **Mapping:**

  | SSH config | Profile field | Notes |
  |---|---|---|
  | `Host` pattern | `name` | Used if it's a valid name (§4); otherwise prompt for one |
  | `IdentityFile` | `ssh_key` | First directive wins |
  | `User` | `username` | Optional |
  | prompted | `git_name`, `git_email` | Required |
  | — | `signing_key` | Not set; use `profile edit` |

- **Edge cases:**
  - File not found → `cannot open SSH config: <path>: no such file or directory`.
  - Parse error → `cannot parse SSH config: <path>: <detail>`.
  - No usable hosts → `No SSH host entries found in <path>.`
  - Name already exists → `Profile "<name>" already exists, skipping.`
  - No `IdentityFile` → import it anyway, with the warning
    `No IdentityFile set for "<name>" — SSH key will be empty.`

## 7. Hooks

`core.hooksPath` is global, and it **replaces** each repo's `.git/hooks`. So every gids
shim chains to the hook that would otherwise have run:

```sh
#!/bin/sh
gids='/absolute/path/to/gids'          # baked in, so GUI apps with a minimal PATH work
if [ -x "$gids" ]; then
	"$gids" hook-exec pre-commit "$@" || exit $?
else
	echo "gids: $gids not found; skipping gids pre-commit hook" >&2
fi
h="$(git rev-parse --git-common-dir)/hooks/pre-commit"   # or "$prev/pre-commit"
if [ -x "$h" ]; then exec "$h" "$@"; fi
exit 0
```

A missing binary never blocks a commit. Every invocation re-renders the path, so moving
the binary is healed on the next gids run. `hook-exec` also fails open on its own errors,
including panics.

- Repos that set a **local** `core.hooksPath` (husky etc.) bypass gids entirely.
  That's a known limitation, and `status` warns about it.
- `hook-exec` reads answers from `/dev/tty` (hooks get stdin = `/dev/null`). If `/dev/tty`
  can't be opened, the hook is **non-interactive**: it prints warnings to stderr and exits 0.
- `GIDS_TTY=<path>` overrides `/dev/tty`, as a test seam.
- `GIDS_SKIP_HOOKS=1` makes `hook-exec` a no-op (used by `gids amend`).

### 7.1 Interactive UI
- **Pickers are Bubble Tea lists:** arrow keys (or Ctrl-N/Ctrl-P) to move, PgUp/PgDn, type to
  filter, Enter to select. In multi-select, Space toggles. Esc first clears the filter, then
  cancels. *(D7: no j/k, because they would collide with type-to-filter.)* Each row shows the profile name and email.
  The header always lists every key that works at that moment (Ctrl-P/N; PgUp/PgDn only
  when the list overflows; "esc clear filter" vs "esc cancel"), so nothing must be discovered.
  - **Profile picker:** used by the guard (§8, "n" path, with a final `+ New profile` row)
    and by `gids use` with no argument.
  - **Co-author picker** (multi-select, Space to toggle): used by `gids amend -c` (§9.2).
- **In a real terminal, every prompt is a small Bubble Tea program**, including yes/no and
  text input. The UI never mixes line-reading and raw-mode TUI on the same terminal, because
  a background line reader could steal the TUI's keystrokes. *(D6)*
- **`profile add` / `edit` is one form** (Tab/↑↓ between fields, live "✓ file exists" hint
  for the SSH key). `gids with` with no arguments, in a terminal, is the co-author
  checklist, with a final "+ Someone not listed…" row.
- **Styling:** Lip Gloss for ✓ success (green), ⚠ warning (yellow), ✗ error (red), and dimmed
  hints. Use one shared style set in the UI package, not inline styles per command.
- **I/O:** inside hooks, both input and output use the `/dev/tty` handle
  (`tea.WithInput`/`tea.WithOutput`, plus a Lip Gloss renderer bound to it). Colour follows
  that handle and respects `NO_COLOR`.
- **Fallback:** when the input isn't a terminal (`GIDS_TTY` pointing at a file, piped stdin,
  or `TERM=dumb`), pickers fall back to a numbered list read line by line. This is the path
  integration tests drive.
- **Cancel** semantics are defined in §7.2.

### 7.2 Robustness

**Keys and signals.** These apply the same way in pickers, line prompts, and the numbered fallback:

| Input | Meaning | In a normal command | In the guard (hook) |
|---|---|---|---|
| **Ctrl-C** / SIGINT / SIGTERM | "Stop everything" | Print `Cancelled.`, save nothing, exit 130 | Save nothing, exit 130, so **git aborts the commit** |
| **Ctrl-D** (EOF) / **Esc** | "Skip this question" | Same as Ctrl-C, but exit 1 | Save nothing, commit proceeds unchanged, exit 0 |
| Read error / input closed | Input is gone | Treat as Ctrl-D | Treat as Ctrl-D |
| Terminal closed (SIGHUP) | — | Default: the process ends. Atomic writes keep every file consistent (*D8*) | Same; git dies too |
| **Ctrl-Z** | Suspend | Suspend and restore the TUI on `fg` | Same |
| Window resize | — | Re-layout; truncate long rows with `…`, never wrap a row | Same |
| Invalid input (`abc`, `9` out of range, `maybe`) | — | Short hint, ask again. EOF always ends the loop, so it can never hang (B1) | Same |

- **The terminal is always restored** (cursor, raw mode, alt screen), including on panic
  (Bubble Tea's recovery) and on signals.
- **Ctrl-C inside a hook:** git's process group also gets SIGINT. gids must still exit
  promptly and never leave a half-written file behind.

**Files.**
- **Atomic writes:** write a temp file in the same directory, `fsync`, then `rename`. A crash
  or Ctrl-C mid-write leaves the old file or the new one, never a mix.
- **Symlinks** (dotfile managers often symlink `~/.gitconfig`): write to the resolved
  target, keep the link and the target's permissions.
- **Read close to the write.** `config.yaml` is loaded right before a change, never held
  across a prompt, so two terminals can't overwrite each other's edits from minutes earlier.
- **One writer at a time.** Every change to gids' files (`config.yaml`, the generated files,
  the `~/.gitconfig` block), including self-heal, holds an OS advisory lock on
  `~/.config/gids/.lock` from load to last write. So two gids processes at once can't lose
  an update or delete each other's in-flight temp files. The OS releases the lock if gids
  dies, so a crash can never leave gids locked. A command waits up to 10 s for another,
  then errors. Prompts never run under the lock. On a filesystem that can't lock, gids
  proceeds unlocked. (Proven in `verify/tla/GidsFiles.tla`; tested by
  `TestLockPreventsLostUpdates` and `TestLockProtectsInFlightWrites`.)
- **`~/.gitconfig` isn't locked against `git config --global`** (decided). gids writes it
  only in `install`/`uninstall`, so a collision needs both within milliseconds. The worst
  case is that one of the two edits is lost (re-run it); the file can't be corrupted,
  since both write atomically. Taking git's own `.lock` would be worse: if gids were
  killed mid-write, the stale lock would block every `git config --global` until it was
  deleted by hand, because git never expires its locks.
- **Write order:** `config.yaml` first (it's the source of truth), then the generated files.
- **Self-healing renders.** Every gids invocation, *including the guard*, first checks that
  the generated files equal `render(config.yaml)`, and rewrites them if not. So an
  interrupted write is repaired before it can matter. If the guard's repair changes which
  identity applies to the current repo, it aborts like §8 ("re-run your commit").
- **Corrupt or unknown YAML** → error naming the file and line, and gids never rewrites it.
  Unknown keys are errors (to catch typos). The guard fails open.
- `config.yaml` is mode 0600. Generated files are 0644 and hooks 0755.

**Values.**
- **Trim surrounding whitespace.** Reject control characters and newlines in every field:
  a newline in a value could inject extra git-config lines.
- **Escape values for git config** (quotes, backslashes, `#`, `;`). Shell-quote paths in
  `sshCommand`. Test with spaces, quotes, `$`, and unicode.
- **Emails:** require exactly one `@`, with something on each side, and no spaces or `<>`.
  No further RFC validation.

**Environment.**
- **Missing `git`, git older than 2.32, or no `HOME`** → one clear error.
- **Not a repo, bare repo, no commits yet** → clear errors from `use`, `amend`, `with`.
- **Worktrees and submodules:** a submodule's git dir lives under its parent's
  `.git/modules/`, so it inherits the parent's rule. Document this.
- **Rebase, merge, or cherry-pick in progress:** the guard still runs normally.
  `amend` refuses until it finishes.

**History rewriting (`amend`).** *(D2: implemented with commit plumbing instead of rebase.)*
- **Refuse while a rebase, merge, cherry-pick, revert or bisect is in progress.** A dirty
  working tree is fine: trees are reused exactly, so the index and worktree are never
  touched.
- **Nothing is visible until one compare-and-swap ref update.** An interruption at any
  earlier point leaves the repo exactly as it was. If the branch moved meanwhile, the update
  fails and nothing changes.
- **Every rewritten commit is re-read and checked** (tree, parents, identity, dates, message,
  encoding) before the ref moves.
- **Afterwards, print `To undo: git reset --keep <old sha>`**; the old sha is also saved as
  `refs/gids/amend-backup`.
- Never push. Print a reminder when a force-push is needed.

## 8. Commit guard (`pre-commit`)

**Constraint (verified):** git settles the commit's author *before* pre-commit runs,
and passes it to the hook as `GIT_AUTHOR_NAME` / `GIT_AUTHOR_EMAIL`. Changing config in
the hook can't change the commit in progress. So when the identity has to change, the
guard fixes the config and **aborts** (exit 1), telling the user to re-run. Pre-commit
runs before the editor opens, so nothing has been typed yet. A `-m` message is still in
shell history.

Inputs: the commit's identity from `GIT_AUTHOR_*`, and the mapped profile from
`git config --get gids.profile`.

**Mapped, identity matches** → silent, exit 0.

**Mapped, identity differs** (a local `.git/config` override, a stray global `[user]`
after the gids block, `--author`, or env vars):
```
⚠ This commit is authored as brian@personal.com, but this repo is mapped to Work (brian@company.com).
  Source: .git/config
Remove the local override and re-run? [Y/n]
```
- Override is in the local `.git/config` → Y: unset local `user.name`/`user.email`, then
  exit 1 with `Fixed — re-run your commit.` n: exit 0.
- Override comes from anywhere else → print where it comes from, exit 0.

**Unmapped:**
```
Committing as Brian Nguyen <bqn@andrew.cmu.edu> in ~/projects/foo (no gids rule).
Is this right? [Y/n]
```
- **Y**, identity matches profile P → `Always use P for ~/projects/foo? [Y/n]` → save the rule if Y; exit 0.
- **Y**, no matching profile → `Save as a new profile? Name (Enter to skip):` → create it,
  offer the rule, exit 0. This repo's identity didn't change, so the commit proceeds.
- **n** → choose a profile in the picker (§7.1), or `+ New profile` (wizard with name/email prefilled).
  Save the rule, then exit 1: `This repo now uses P. Re-run your commit.`
- If no profiles exist, `n` goes straight to the wizard.
- Unmapped repos prompt on every commit. Saving a rule is how you silence it.

**Non-interactive** (no tty): unmapped → one stderr line
`gids: no rule for ~/projects/foo; committing as X`; mismatch → the warning. Exit 0.

**Errors** (unreadable config, git failures) → one stderr line, exit 0.

## 9. Pairing and amend

### 9.1 `gids with` (+ `commit-msg`, `post-commit`)
- `gids with <target>...`: `target` is a profile name or `"Name <email>"`, resolved to
  `Name <email>` **at `with` time** and stored in the repo's local config as the
  multi-valued `gids.coauthor`. The session is scoped to the repo, shared by its worktrees,
  and goes away with the repo.
- `--once` also sets `gids.coauthorOnce = true`. `--clear` unsets both. No args → list.
- Unknown profile → `Profile 'alice' not found. Use: gids with "Alice Smith <alice@example.com>" or gids profile add`.
- **`commit-msg`** *(D1, replacing prepare-commit-msg)*: if the message isn't empty (after
  `git stripspace --strip-comments`), run `git interpret-trailers --in-place --if-exists
  addIfDifferent --trailer "Co-authored-by: …" "$1"`. It's safe on amend, `-v`, merges and
  repeated runs.
  - **Why not prepare-commit-msg:** it runs before the editor, so its trailer made an empty
    message non-empty, and git created a commit whose only content was the trailer instead
    of aborting (verified).
  - **Cost:** `--no-verify` also skips co-authors.
- `post-commit`: if `gids.coauthorOnce`, clear the session. This runs only after a
  successful commit, so an aborted commit keeps the `--once` co-author.

### 9.2 `gids amend [-n N | --since <rev>] [--author <profile>] [--add-coauthor T] [--remove-coauthor E] [-c]`

**Which commits get rewritten:**
- No flag → `HEAD` only.
- `-n N` → the last N commits.
- `--since <rev>` → every commit reachable from `HEAD` but not from `<rev>` (git's `<rev>..HEAD`).
  `<rev>` is any commit-ish: a branch (`main`, `origin/main`), a tag, a full or short sha,
  `HEAD~3`. For a feature branch, `--since main` means exactly "this branch's commits",
  even if `main` has moved on since. Commits are rewritten in place on their existing
  parents, so nothing gets rebased onto a newer `main`.

**Validation.** Each of these is a clear error that names what was typed, and nothing is changed:

| Case | Error |
|---|---|
| `-n` and `--since` both given | `use either -n or --since, not both` |
| `-n` < 1, or not a number | `-n must be a positive integer` |
| `-n` > number of commits on HEAD | `only K commits exist; use -n K` (K = all of them, which uses `--root`) |
| `<rev>` doesn't resolve (`git rev-parse --verify --end-of-options <rev>^{commit}`) | `unknown revision "<rev>"` |
| A short sha matches several commits | git's ambiguity message, passed through |
| `<rev>..HEAD` is empty (rev is HEAD or ahead of it) | `no commits after <rev> on this branch; nothing to amend` |
| No common ancestor (unrelated histories) | `<rev> shares no history with HEAD` |
| `<rev>` starts with `-` | rejected by `--end-of-options` (no flag injection) |
| Detached HEAD | allowed, with a note that no branch will point at the result except HEAD |

**Before rewriting**, show the exact list and confirm. `-y` skips the confirmation (*D11*).
```
Re-author 15 commits on dev-2026.11 as Brian Nguyen <nguyenbriankhoi@gmail.com>:
  8d11424  Update dependencies for current Discord…   bqn@andrew.cmu.edu
  …
  2a49961  Add a systemd service for running on a…    bqn@andrew.cmu.edu
⚠ 15 of these are on origin/dev-2026.11 — you'll need to force-push.
Proceed? [y/N]
```
For history rewrites the default is **No**.
- Author defaults to the repo's mapped profile; with no mapping it's an error.
- **Mechanism (*D2*):** walk the commits oldest-first. Build each with `git commit-tree` from
  the same tree, the same parents (mapped to their rewritten versions), and the same author
  and committer **dates** and message. Verify each one, then make one CAS
  `git update-ref <branch> <new> <old>`. Merges keep their topology, there's no replay, and
  there can be no conflicts.
- **Both author and committer become the profile (*D9*).** In the PR #8 case the committer
  email was wrong too.
- **Published history:** warn if any of the commits are reachable from a `refs/remotes/*` ref.
- Hooks are skipped (`GIDS_SKIP_HOOKS=1`; plumbing runs no hooks anyway). **Guarantee:**
  without co-author flags, the message is byte-for-byte unchanged.
- **Already correct:** if every commit already has the target identity, amend says so and
  changes nothing.
- `--add-coauthor`: append the trailer with `git interpret-trailers --if-exists addIfDifferent`.
- `--remove-coauthor <email>`: drop `Co-authored-by:` lines with that email.
- `-c` / `--coauthor`: open the co-author picker (§7.1), pre-checked with the commit's
  current `Co-authored-by:` trailers, and apply the difference as adds/removes.
- Co-author flags work only with N=1.

## 10. Decisions

Decided:
1. When the guard changes identity, it fixes the config and aborts so the commit is re-run (§8).
2. Non-interactive commits (IDE, scripts) fail open with a warning (§8).
3. The config dir on macOS is `~/.config/gids` (§3).
4. No remote-URL rules. Org prefixes don't reliably identify repos, and new repos have
   no remote. Rules are directories only.
5. Full TUI: Bubble Tea pickers + Lip Gloss styling (§7.1).

## 11. Invariants: states that must be impossible to break
These hold after **every** gids command, whether it succeeds, fails, or is interrupted at any
point (Ctrl-C or `kill -9`). After an interrupt they may hold only once the next gids
invocation has self-healed (§7.2). Each is checked by one shared `checkInvariants` test
helper.

| # | Invariant |
|---|---|
| I1 | `config.yaml` parses and validates: profile names are unique and valid, required fields are present, every rule points to an existing profile, and there are no duplicate rules. A command can't save a config that fails this. |
| I2 | The generated files equal `render(config.yaml)`: none missing, and no extra files *with names gids generates* (`gitconfig`, `profiles/<name>.gitconfig`, `hooks/<gids hook>`). Any other file in those folders (Finder's `.DS_Store`, your own notes) is never touched. |
| I3 | `~/.gitconfig` has either zero gids blocks or exactly one well-formed block (begin and end). Everything outside the block is byte-identical to before gids touched it. |
| I4 | Installed (the block is present) ⇔ the generated files and executable hooks exist. Not installed ⇔ no generated files, and `~/.gitconfig` (including `core.hooksPath`) is exactly what it was before install. |
| I5 | gids only writes a repo's `.git/config` for `gids.*` keys, or to unset `user.name`/`user.email` after an explicit "yes". |
| I6 | `amend` either finishes fully or leaves the repo exactly at its original HEAD. There is no partial state (D2: a single CAS ref update). |
| I7 | No prompt can wait forever without input, and no prompt can loop forever. |
| I8 | `install` and `uninstall` are idempotent, and `install` then `uninstall` returns every file and setting it touched to its original bytes and values. |

**How the invariants are tested and proven** (`make verify` runs it all; see `verify/README.md`):
- **Crash at every step:** test builds (`-tags gidstest`, `internal/testhook`) number every
  file operation, lock step and git command. `TestCrashAtEveryStep` kills each mutating
  command at *each* of its steps (`SIGKILL`), checks I1/I3 right away, and checks I1–I4 (I6
  for amend) after the next gids command. Release builds compile those hooks to empty
  functions.
- **Event-gated interleavings:** a test build can pause at an exact step, so two-process
  races run deterministically, each with the lock disabled as a negative control.
- **Exhaustive state search:** every state a small domain can reach, with every operation
  tried from every state, checked against a model and against real git's `includeIf`.
- **Model checking (TLA+/TLC):** the file protocol, amend, the guard, and `with --once`.
  Each invariant has a negative control (the model deliberately broken) that TLC must
  catch. `TestTracesFollowTheModeledProtocol` checks that the real code's step order is
  the one the models assume.
- **Static checks:** every file change goes through `fsx` and is a numbered step; the
  only program run is git; no script does a recursive delete.
- **Fuzzing** (Go native `testing.F`):
  - *round-trip*: rendering a value and reading it back with
    `git config --file <f> --get` gives the original, for arbitrary strings;
  - rule normalization is idempotent;
  - the marker-block edit preserves everything outside the block;
  - YAML loading never panics.
- **Model-based sequences:** random sequences of `profile add/edit/delete`, `rule add/remove`,
  `use`, `install`/`uninstall`, with `checkInvariants` after every step.

### 11.1 Development safety (gids modifies real files)
- **Tests never touch the real home.** Every test runs with `HOME`, `XDG_CONFIG_HOME`,
  `GIT_CONFIG_GLOBAL` set to temp directories, plus `GIT_CONFIG_NOSYSTEM=1`. A `TestMain`
  in each package aborts the run if any of these resolves inside the real user's home.
- **Manual runs during development** happen only in a sandbox shell (`make sandbox`: temp
  `HOME`, built binary first on `PATH`), never against the real `~/.gitconfig`.
- **First real install:** back up `~/.gitconfig`, the gids dir, and
  `git config --global core.hooksPath`; show the exact diff gids will make; proceed only on
  Brian's go-ahead.
- **First real `amend`** (discord-music-bot#8): only after the acceptance test passes. Run
  it on a fresh clone first, compare the result, then on the real checkout. The force-push
  is Brian's call.

## 12. Testing
Tests come before gids is used on a real repo. There are three layers, and every behavior
in this spec, including every row of §7.2, has a test at the lowest layer that can
honestly check it.

1. **Unit** (pure functions, no processes):
   - rendering: profile → INI, rules → ordered `includeIf`;
   - path normalization, value escaping and quoting, input validation;
   - the marker-block edit;
   - the prompt and fallback loops: EOF, invalid input, defaults;
   - TUI models: drive `Update` with key messages (arrows, filter text, Enter, Esc, Ctrl-C,
     resize) and assert on the selection and `View()`.
2. **Integration** (real `git`, temp `HOME`, `GIT_CONFIG_NOSYSTEM=1`, built binary):
   - `install`/`uninstall` round-trip, including a symlinked `~/.gitconfig` and a
     pre-existing `core.hooksPath`;
   - `includeIf` actually applies, from subdirectories and worktrees;
   - hook chaining to the repo's own hooks;
   - every guard path in §8, through a real `git commit`, with `GIDS_TTY` scripted answers.
     Assert on `git log --format='%an <%ae>'`, the exit status, and `config.yaml`. (v1's
     guard passed its unit tests and failed every real commit.)
   - `with` trailers, and `--once` surviving an aborted commit;
   - `amend` for N=1, N>1, `--root`, merges, a dirty tree, the backup ref, and author dates kept.
3. **End-to-end** (real pseudo-terminal via `creack/pty`, built binary): type into the
   actual TUI the way a person would. Cover:
   - picking with arrows and with filter text;
   - Ctrl-C during a guard prompt aborts the commit, with exit 130 and no files changed;
   - Ctrl-D and Esc skip, and the commit proceeds;
   - the terminal state is restored afterwards;
   - resize mid-picker;
   - `NO_COLOR`.

**Acceptance scenario: the wrong-identity PR.** This mirrors
`briankhoi/discord-music-bot#8`. Build a fixture repo with `main` plus a branch of 15
commits authored `Brian Nguyen <bqn@andrew.cmu.edu>`, containing a variety of messages
(some with trailers). Run `gids amend` to re-author the branch as
`Brian Nguyen <nguyenbriankhoi@gmail.com>`. Assert:
- all 15 authors are fixed;
- author dates, messages, trailers, and trees are identical (compare `git log --format=%ad%n%B` and tree hashes);
- `main` is untouched;
- the backup ref and undo hint exist.

Only after this passes is gids run on the real repo, and the force-push stays a manual step.
- TUI models are tested by sending key messages to `Update` and asserting on the
  selection and `View()` output, with no real terminal. Integration tests use the
  numbered fallback (§7.1).
- No test touches the real `~/.gitconfig`, `~/.ssh`, or gids dir.
- CI: `gofmt -l` (must be empty), `go vet`, `go test -race`, and govulncheck (§2.1).

## 13. Roadmap
1. **Foundations:** pin toolchain go1.27.3, add govulncheck + gofmt + `-race` to CI, fix
   the prompt EOF bug, move the gids dir, drop dead code. Then build the test harness
   (temp-`HOME` git helper, `GIDS_TTY`, pty driver), so every later step is test-first.
2. **Render engine:** profile/rule rendering, `~/.gitconfig` block, `install`/`uninstall`;
   rewrite `use`, `rule`, `status` on top of it. Remove the shell hook, `check`, and per-repo `Apply`.
3. **Hooks + guard v2:** shims with chaining, `hook-exec`, the §8 guard, integration tests.
4. **Demo:** `examples/demo.sh` in a temp `HOME`, driven through `GIDS_TTY`.
5. **Pairing + amend:** §9.
6. **TUI + polish:** Bubble Tea pickers and Lip Gloss styles (§7.1), then the README (install,
   `includeIf` explained, limitations).

## 14. Migrating from v1
- v1 `gids use`/`check` wrote identity into each repo's `.git/config`, and that overrides
  `includeIf`. Those repos show up as "mapped, identity differs, source: .git/config"
  in the guard and `status`, and the fix is offered there. `gids use` also offers it.
- v1 rules like `~/work/*` become `~/work/`. Edit by hand.
- Remove the old shell hook block (`# gids:hook:begin … end`) from `~/.zshrc`.
