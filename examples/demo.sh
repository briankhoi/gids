#!/bin/sh
# gids demo: the common workflows, end to end, in a throwaway HOME.
# Your real ~/.gitconfig, ~/.config and repos are never touched.
#
#   make build && ./examples/demo.sh          (or: GIDS=/path/to/gids ./examples/demo.sh)
set -eu

GIDS=${GIDS:-$(cd "$(dirname "$0")/.." && pwd)/bin/gids}
[ -x "$GIDS" ] || { echo "build first: make build (or set GIDS=/path/to/gids)" >&2; exit 1; }

# ---- sandbox (defensive: never let an empty/real path through) ----
REAL_HOME=$HOME
SANDBOX=$(mktemp -d "${TMPDIR:-/tmp}/gids-demo.XXXXXX")
case "$SANDBOX" in ""|/|"$REAL_HOME"|"$REAL_HOME"/*) echo "refusing unsafe sandbox '$SANDBOX'" >&2; exit 1;; esac
[ -d "$SANDBOX" ] || { echo "sandbox missing" >&2; exit 1; }
trap 'echo; echo "(demo files left in $SANDBOX; delete them whenever you like)"' EXIT

export HOME="$SANDBOX" XDG_CONFIG_HOME="$SANDBOX/.config" GIT_CONFIG_GLOBAL="$SANDBOX/.gitconfig" \
	GIT_CONFIG_NOSYSTEM=1 NO_COLOR=1 GIT_TERMINAL_PROMPT=0
unset GIT_DIR GIT_WORK_TREE GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL 2>/dev/null || true
ANSWERS="$SANDBOX/answers" # scripted answers for the commit guard (it reads them instead of /dev/tty)
export GIDS_TTY="$ANSWERS"
gids() { "$GIDS" "$@"; }

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
run() { printf '$ %s\n' "$*"; "$@" || printf '(exit %s)\n' "$?"; }
show() { printf -- '--- %s\n' "${1#"$SANDBOX"/}"; sed 's/^/    /' "$1"; }
answer() { printf '%s\n' "$@" > "$ANSWERS"; }
edit() { echo "$2" >> "$1/notes.txt"; git -C "$1" add notes.txt; }

# A global identity, like most people have.
printf '[user]\n\tname = Brian Nguyen\n\temail = bqn@andrew.cmu.edu\n' > "$GIT_CONFIG_GLOBAL"

step "1. Install once (adds one block to ~/.gitconfig)"
run gids install
show "$GIT_CONFIG_GLOBAL"

step "2. Add your identities"
run gids profile add --name School --git-name "Brian Nguyen" --email bqn@andrew.cmu.edu
run gids profile add --name Personal --git-name "Brian Nguyen" --email nguyenbriankhoi@gmail.com --ssh-key "~/.ssh/id_personal"
run gids profile list

step "3. Map folders: every repo inside uses that identity, at any depth"
run gids rule add ~/school School
run gids rule add ~/code Personal
run gids rule list
show "$XDG_CONFIG_HOME/gids/gitconfig"
show "$XDG_CONFIG_HOME/gids/profiles/Personal.gitconfig"

step "4. Commit in a mapped repo: git applies the identity itself (terminal, IDE, scripts)"
mkdir -p ~/code/bot/src && git -C ~/code/bot init -q -b main
edit ~/code/bot "hello"
(cd ~/code/bot/src && run git commit -q -m "first commit" && run gids status)
run git -C ~/code/bot log -1 --format='author: %an <%ae>'

step "5. Unmapped repo: the guard asks before you commit as the wrong person"
mkdir -p ~/Downloads/fork && git -C ~/Downloads/fork init -q -b main
edit ~/Downloads/fork "fix"
answer n 2   # "Is this right?" → no; pick profile 2 (Personal)… then the commit stops
(cd ~/Downloads/fork && run git commit -q -m "fix typo")
answer ""    # re-run: now mapped, silent
(cd ~/Downloads/fork && run git commit -q -m "fix typo" && run git log -1 --format='author: %an <%ae>')

step "6. A stray local override is caught and fixed"
git -C ~/code/bot config user.email wrong@example.com
edit ~/code/bot "more"
answer y     # "Remove the local override and re-run?"
(cd ~/code/bot && run git commit -q -m "oops")
answer ""
(cd ~/code/bot && run git commit -q -m "now right" && run git log -1 --format='author: %an <%ae>')

step "7. Pairing: co-authors on every commit until cleared (or --once)"
(cd ~/code/bot && run gids with "Alice Smith <alice@example.com>")
edit ~/code/bot "pairing"
(cd ~/code/bot && run git commit -q -m "pair on the queue" && run git log -1 --format=%B && run gids with --clear)

step "8. Fix commits made with the wrong identity (the PR #8 case)"
git -C ~/code/bot checkout -q -b feature
for i in 1 2 3; do
	edit ~/code/bot "feature $i"
	git -C ~/code/bot -c user.email=bqn@andrew.cmu.edu commit -q --no-verify -m "feature $i"
done
git -C ~/code/bot config --unset user.email || true
run git -C ~/code/bot log --format='%h %ae %s' main..feature
(cd ~/code/bot && printf 'y\n' | run gids amend --since main)
run git -C ~/code/bot log --format='%h %ae %s' main..feature

step "9. Uninstall: ~/.gitconfig goes back to exactly what it was"
run gids uninstall
show "$GIT_CONFIG_GLOBAL"
