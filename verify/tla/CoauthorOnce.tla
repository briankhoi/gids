---------------------------- MODULE CoauthorOnce ----------------------------
(***************************************************************************)
(* `gids with --once` (SPEC §9.1, D1): the co-author trailer goes on the   *)
(* next successful commit only, and an empty editor message must still     *)
(* abort the commit.                                                       *)
(*                                                                         *)
(* A commit attempt: [prepare-commit-msg] → editor → commit-msg hook →     *)
(* later checks (other hooks, git's empty-message rule) → commit created → *)
(* post-commit hook. The attempt can be abandoned at any point (Ctrl-C,    *)
(* another hook failing), and git can be killed between creating the       *)
(* commit and running post-commit.                                         *)
(***************************************************************************)
EXTENDS Sequences, Naturals

CONSTANTS MaxAttempts,
          PrepareMsgTrailer,  \* TRUE: negative control (add the trailer before the editor)
          ClearInCommitMsg,   \* TRUE: negative control (end the session in commit-msg)
          AllowPostCrash      \* model git being killed before post-commit

VARIABLES session, pc, msg, trailer, commits, attempts, crashedPost

vars == <<session, pc, msg, trailer, commits, attempts, crashedPost>>

Init ==
    /\ session = TRUE          \* `gids with --once alice` just ran
    /\ pc = "idle" /\ msg = "none" /\ trailer = FALSE
    /\ commits = <<>> /\ attempts = 0 /\ crashedPost = FALSE

Begin ==
    /\ pc = "idle" /\ attempts < MaxAttempts
    /\ attempts' = attempts + 1
    /\ trailer' = (PrepareMsgTrailer /\ session)   \* the old design: before the editor
    /\ msg' = "none"
    /\ pc' = "editor"
    /\ UNCHANGED <<session, commits, crashedPost>>

Editor ==   \* the user writes a message, or leaves it empty
    /\ pc = "editor"
    /\ msg' \in {"empty", "text"}
    /\ pc' = "commitmsg"
    /\ UNCHANGED <<session, trailer, commits, attempts, crashedPost>>

CommitMsg == \* gids' commit-msg hook: add the trailer unless the message is empty
    /\ pc = "commitmsg"
    /\ trailer' = (trailer \/ (session /\ msg = "text"))
    /\ session' = IF ClearInCommitMsg THEN FALSE ELSE session
    /\ pc' = "checks"
    /\ UNCHANGED <<msg, commits, attempts, crashedPost>>

Checks ==   \* git refuses a message with no content (trailers count as content)
    /\ pc = "checks"
    /\ IF msg = "empty" /\ ~trailer
         THEN pc' = "idle" /\ UNCHANGED commits                   \* "Aborting commit due to empty commit message."
         ELSE /\ commits' = Append(commits, [empty |-> msg = "empty", trailer |-> trailer])
              /\ pc' = "post"
    /\ UNCHANGED <<session, msg, trailer, attempts, crashedPost>>

PostCommit == \* gids' post-commit hook: a --once session ends after a commit
    /\ pc = "post"
    /\ session' = FALSE
    /\ pc' = "idle"
    /\ UNCHANGED <<msg, trailer, commits, attempts, crashedPost>>

Abandon ==  \* Ctrl-C in the editor, another hook fails, …
    /\ pc \in {"editor", "commitmsg", "checks"}
    /\ pc' = "idle"
    /\ UNCHANGED <<session, msg, trailer, commits, attempts, crashedPost>>

KilledBeforePost ==
    /\ AllowPostCrash /\ pc = "post"
    /\ pc' = "idle" /\ crashedPost' = TRUE
    /\ UNCHANGED <<session, msg, trailer, commits, attempts>>

Done == pc = "idle" /\ attempts = MaxAttempts

Next == Begin \/ Editor \/ CommitMsg \/ Checks \/ PostCommit \/ Abandon
        \/ KilledBeforePost \/ (Done /\ UNCHANGED vars)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
\* D1: an empty message still aborts; gids never turns it into a commit
\* whose message is only the trailer.
NoTrailerOnlyCommit == \A i \in DOMAIN commits : ~commits[i].empty

\* The session is never used up without a commit carrying the trailer: an
\* abandoned attempt doesn't consume it.
OnceNotLost == ~session => (Len(commits) >= 1 /\ commits[1].trailer)

\* Only the first commit carries it (unless git was killed before post-commit).
OnceNotRepeatedStrict == \A i \in DOMAIN commits : i > 1 => ~commits[i].trailer

\* Known gap (once_neg_known_gap_postcrash.cfg shows it): if git is killed
\* after creating the commit but before post-commit, the session survives
\* and the next commit carries the trailer too. Accepted: the window is one
\* process exit, and the extra trailer is visible and easy to remove.
OnceNotRepeated == ~crashedPost => OnceNotRepeatedStrict
=============================================================================
