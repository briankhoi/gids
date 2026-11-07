------------------------------- MODULE Amend -------------------------------
(***************************************************************************)
(* `gids amend` (SPEC §9.2, D2) racing a concurrent `git commit` on the    *)
(* same branch (an IDE, another terminal), with a crash at any step.       *)
(*                                                                         *)
(* gids writes new commit objects (commit-tree: invisible until a ref      *)
(* points at them), saves the old head in refs/gids/amend-backup, then     *)
(* moves the branch with ONE compare-and-swap update-ref. The real step    *)
(* order is checked by TestTracesFollowTheModeledProtocol (P6).            *)
(*                                                                         *)
(* A commit is a record [id, by]: by = "school" (wrong identity) or        *)
(* "personal". The branch is the sequence of commits above main.           *)
(***************************************************************************)
EXTENDS Sequences, Naturals, FiniteSets

CONSTANTS N,          \* commits to re-author
          UseCAS,     \* FALSE: negative control (unconditional ref update)
          PerCommit,  \* TRUE: negative control (move the branch after each commit, like a rebase)
          OffByOne,   \* TRUE: negative control (the plan misses the oldest commit)
          SkipBackup  \* TRUE: negative control (no refs/gids/amend-backup)

Orig == [i \in 1..N |-> [id |-> i, by |-> "school"]]
Rewritten(s) == [i \in 1..Len(s) |-> [id |-> s[i].id,
                    by |-> IF OffByOne /\ i = 1 THEN s[i].by ELSE "personal"]]

VARIABLES branch, backup, apc, old, k, userCommits, wpc

vars == <<branch, backup, apc, old, k, userCommits, wpc>>

Init ==
    /\ branch = Orig
    /\ backup = <<>>
    /\ apc = "plan"        \* amend's program counter
    /\ old = <<>>
    /\ k = 0
    /\ userCommits = {}    \* ids of commits the other writer made
    /\ wpc = "ready"       \* the other writer: one `git commit`

\* --- gids amend ----------------------------------------------------------
Plan ==       \* read HEAD and the commits to rewrite
    /\ apc = "plan"
    /\ old' = branch
    /\ apc' = "write"
    /\ UNCHANGED <<branch, backup, k, userCommits, wpc>>

Write ==      \* commit-tree for each commit: new objects, no ref changes
    /\ apc = "write"
    /\ apc' = "backup"
    /\ UNCHANGED <<branch, backup, old, k, userCommits, wpc>>

Backup ==     \* update-ref refs/gids/amend-backup <old head>
    /\ apc = "backup"
    /\ backup' = IF SkipBackup THEN backup ELSE old
    /\ apc' = IF PerCommit THEN "step" ELSE "move"
    /\ UNCHANGED <<branch, old, k, userCommits, wpc>>

Move ==       \* update-ref <branch> <new> <old>: compare-and-swap
    /\ apc = "move"
    /\ IF ~UseCAS \/ branch = old
         THEN branch' = Rewritten(old)
         ELSE UNCHANGED branch       \* branch moved meanwhile: refuse, change nothing
    /\ apc' = "done"
    /\ UNCHANGED <<backup, old, k, userCommits, wpc>>

\* Negative control: a rebase-like amend moves the branch one commit at a
\* time (rewritten prefix; the rest is replayed later). A crash halfway
\* doesn't mix identities, it drops the commits not yet replayed: TLC
\* reports NoLostCommit (amend_neg_per_commit.cfg).
StepOne ==
    /\ apc = "step" /\ k < Len(old)
    /\ branch' = Rewritten(SubSeq(old, 1, k + 1))
    /\ k' = k + 1
    /\ apc' = IF k + 1 = Len(old) THEN "done" ELSE "step"
    /\ UNCHANGED <<backup, old, userCommits, wpc>>

Crash ==
    /\ apc \notin {"done", "crashed"}
    /\ apc' = "crashed"
    /\ UNCHANGED <<branch, backup, old, k, userCommits, wpc>>

\* --- the other writer: `git commit` appends a commit to the branch ------
UserCommit ==
    /\ wpc = "ready"
    /\ branch' = Append(branch, [id |-> 100, by |-> "personal"])
    /\ userCommits' = {100}
    /\ wpc' = "done"
    /\ UNCHANGED <<backup, apc, old, k>>

Done == apc \in {"done", "crashed"} /\ wpc = "done"

Next == Plan \/ Write \/ Backup \/ Move \/ StepOne \/ Crash \/ UserCommit
        \/ (Done /\ UNCHANGED vars)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
Ids(s) == {s[i].id : i \in DOMAIN s}

\* No commit is ever lost: everything the user committed, and every original
\* commit (in either identity), is still on the branch.
NoLostCommit == (userCommits \cup Ids(Orig)) \subseteq Ids(branch)

\* I6: the re-authored range is all-or-nothing: the original commits are
\* either all still the school identity or all personal, never a mix.
AllOrNothing ==
    LET ours == {i \in DOMAIN branch : branch[i].id \in Ids(Orig)}
    IN \/ \A i \in ours : branch[i].by = "school"
       \/ \A i \in ours : branch[i].by = "personal"

\* Undo is always possible: once the branch holds rewritten commits, the
\* backup ref holds the head from before.
UndoAvailable ==
    (\E i \in DOMAIN branch : branch[i].id \in Ids(Orig) /\ branch[i].by = "personal")
        => backup = old
=============================================================================
