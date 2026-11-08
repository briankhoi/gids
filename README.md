# gids — Git Identity Swap

Never commit, push, or sign as the wrong person again. `gids` keeps your git
identities (Work, Personal, School…) and maps directories to them. **Git itself**
applies the right one everywhere — terminal, IDE, scripts — and gids guards every
commit in case you're somewhere it doesn't know yet.

```sh
gids install                     # once per machine
gids profile add                 # once per identity (a form; or pass flags)
gids rule add ~/work Work        # every repo under ~/work, at any depth
cd ~/code/side-project && gids use Personal   # or map just this repo
```

## How it works

gids writes git's own [`includeIf`](https://git-scm.com/docs/git-config#_conditional_includes)
config for you. There's no shell hook, and nothing is rewritten when you `cd`:

```ini
# ~/.config/gids/gitconfig  (generated)
[includeIf "gitdir:~/work/"]
	path = "profiles/Work.gitconfig"   # user.name, user.email, core.sshCommand, signing key…
```

`gids install` adds one marked block to `~/.gitconfig` that includes that file and sets
`core.hooksPath` to gids' hooks. The hooks chain to your repos' own hooks, so husky,
pre-commit and friends keep working. `gids uninstall` puts `~/.gitconfig` back
byte-for-byte.

### The commit guard
In a repo no rule covers, `git commit` asks first:

```
Committing as Brian Nguyen <bqn@andrew.cmu.edu> in ~/Downloads/fork (no gids rule).
Is this right? [Y/n] n
Commit as which profile?  (↑/↓ move · enter select · type to filter · esc cancel)
  ▸ Personal  Brian Nguyen <nguyenbriankhoi@gmail.com>
✓ This repo now uses Personal. Re-run your commit to commit as Personal.
```

- **Answering "n" stops the commit.** Git fixes a commit's author before hooks run, so
  that's the only way to make it right. It happens before your editor opens, so nothing
  you typed is lost.
- **Once a rule exists, commits are silent.** In a mapped repo, gids only speaks up if
  something overrides the profile (e.g. a `user.email` in `.git/config`) and offers to
  remove it.
- **Keys:** Ctrl-C aborts the commit. Esc or Ctrl-D skips the question and commits as-is.
- **No terminal (IDE commits)?** gids prints a one-line warning and never blocks.

### Pairing
```sh
gids with alice "Bob Lee <bob@example.com>"   # Co-authored-by trailers on every commit here
gids with --once alice                         # just the next commit
gids with                                      # in a terminal: pick from a checklist
gids with --clear
```

### Fixing commits made as the wrong person
```sh
gids amend                         # last commit, as this repo's profile
gids amend --since main            # every commit on this branch
gids amend -n 3 --author Personal
gids amend --add-coauthor alice    # or -c to pick co-authors
```

- **Only the identity changes.** Author and committer change; code, messages, dates and
  merges stay byte-identical.
- **One atomic step.** History is rewritten with a single ref update, so an interrupted
  amend changes nothing.
- **Undo:** gids prints the undo command and keeps `refs/gids/amend-backup`.
- **It never pushes.** If the commits were already pushed, push with `--force-with-lease`.

## Commands

| | |
|---|---|
| `gids install` / `uninstall` | add/remove gids in your global git config |
| `gids profile add\|edit\|list\|delete\|import` | manage identities (`import` reads `~/.ssh/config`) |
| `gids rule add <dir> <profile>` / `list` / `remove <dir>` | map directories; the deepest rule wins |
| `gids use [<profile>]` | map the current repo |
| `gids status` | which identity applies here, where it comes from, and any problems |
| `gids with …` | pairing co-authors for this repo |
| `gids amend …` | re-author commits |

Files: `~/.config/gids/` (or `$XDG_CONFIG_HOME/gids`; `%APPDATA%\gids` on Windows).
`config.yaml` is the source of truth, and everything else there is generated.

## Limitations
- **Repos with their own `core.hooksPath`** (some husky setups) bypass the guard.
  `gids status` warns about this. Identity still applies, because that's `includeIf`.
- **A submodule follows its parent repo's rule**, since its git dir lives inside the parent.
- **`git commit --no-verify`** skips the guard and co-author trailers.
- **Needs git ≥ 2.32.**

## Development
```sh
make test        # all tests, with -race (incl. crash-at-every-step and pty end-to-end tests)
make test-short  # skips the slow ones
make verify      # make test + exhaustive state search + TLA+ models (≈7 min; see verify/README.md)
make sandbox     # a shell with a throwaway HOME and the built gids, to try things safely
make vulncheck   # govulncheck on code, modules and the toolchain
./examples/demo.sh
```
Design: [SPEC.md](SPEC.md) · Known v1 bugs and their fixes: [BUGS.md](BUGS.md).
