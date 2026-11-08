------------------------------- MODULE Guard -------------------------------
(***************************************************************************)
(* The pre-commit guard (SPEC §8). Git computes a commit's author BEFORE   *)
(* hooks run (BUGS.md B3), so a hook that repairs the identity can't change *)
(* the commit in progress: it must stop the commit and let the user re-run. *)
(*                                                                         *)
(* One repo; attempts to commit, each interactive (a terminal) or not      *)
(* (an IDE). The environment can tamper with gids' generated files or add  *)
(* a local user.email override between attempts, a bounded number of times.*)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS MaxAttempts,
          Budget,                 \* environment interferences allowed
          HealThenContinue,       \* TRUE: negative control (v1: fix config, let the commit go on)
          FixOverrideThenContinue, \* TRUE: negative control (B3: unset override, let the commit go on)
          SpuriousAbort           \* TRUE: negative control (heal says "re-run" though it fixed nothing)

VARIABLES mapped,   \* does a rule cover the repo?
          genOK,    \* are gids' generated files intact?
          override, \* is there a local user.email override?
          pc, interactive, explicit, author,
          commits,  \* set of records describing each commit made
          attempts, aborts, budget, fixes

vars == <<mapped, genOK, override, pc, interactive, explicit, author, commits, attempts, aborts, budget, fixes>>

Init ==
    /\ mapped \in BOOLEAN
    /\ genOK = TRUE
    /\ override = FALSE
    /\ pc = "idle"
    /\ interactive = TRUE
    /\ explicit = FALSE
    /\ author = "none"
    /\ commits = {}
    /\ attempts = 0
    /\ aborts = 0
    /\ budget = Budget
    /\ fixes = 0

\* What git would use as the author, computed when the commit starts.
GitAuthor(m, g, o) ==
    IF o THEN "override"
    ELSE IF m /\ g THEN "profile"
    ELSE IF m THEN "broken"      \* generated files damaged: wrong or no identity
    ELSE "global"

\* --- the environment ---------------------------------------------------------
Tamper ==
    /\ pc = "idle" /\ budget > 0 /\ genOK
    /\ genOK' = FALSE /\ budget' = budget - 1
    /\ UNCHANGED <<mapped, override, pc, interactive, explicit, author, commits, attempts, aborts, fixes>>

AddOverride ==
    /\ pc = "idle" /\ budget > 0 /\ ~override
    /\ override' = TRUE /\ budget' = budget - 1
    /\ UNCHANGED <<mapped, genOK, pc, interactive, explicit, author, commits, attempts, aborts, fixes>>

\* --- a commit attempt -----------------------------------------------------
Begin ==
    /\ pc = "idle" /\ attempts < MaxAttempts
    /\ interactive' \in BOOLEAN
    /\ explicit' \in BOOLEAN                       \* git commit --author=…
    /\ author' = IF explicit' THEN "explicit" ELSE GitAuthor(mapped, genOK, override)
    /\ attempts' = attempts + 1
    /\ pc' = "heal"
    /\ UNCHANGED <<mapped, genOK, override, commits, aborts, budget, fixes>>

Commit(accepted, warned) ==
    /\ commits' = commits \cup {[author |-> author, mapped |-> mapped, interactive |-> interactive,
                                 accepted |-> accepted, warned |-> warned]}
    /\ pc' = "idle"

Abort ==
    /\ aborts' = aborts + 1
    /\ pc' = "idle"

\* gids' self-heal at the start of the hook.
Heal ==
    /\ pc = "heal"
    /\ IF ~genOK
         THEN /\ genOK' = TRUE /\ fixes' = fixes + 1
              /\ IF HealThenContinue
                   THEN pc' = "check" /\ UNCHANGED <<commits, aborts>>
                   ELSE Abort /\ UNCHANGED commits   \* "repaired… Re-run your commit."
         ELSE IF SpuriousAbort
                THEN Abort /\ UNCHANGED <<genOK, fixes, commits>>
                ELSE /\ pc' = "check" /\ UNCHANGED <<genOK, fixes, commits, aborts>>
    /\ UNCHANGED <<mapped, override, interactive, explicit, author, attempts, budget>>

\* The guard's decision, with the author git already fixed (mirrors
\* preCommit/mismatch/unmapped in internal/cmd/hook.go).
Check ==
    /\ pc = "check"
    /\ CASE author = "explicit" ->                \* --author / GIT_AUTHOR_*: warn only
              Commit(FALSE, TRUE) /\ UNCHANGED <<mapped, override, aborts, fixes>>
         [] mapped /\ author = "profile" ->       \* right identity: silent
              Commit(FALSE, FALSE) /\ UNCHANGED <<mapped, override, aborts, fixes>>
         [] mapped /\ ~interactive ->             \* IDE: one-line warning, never block
              Commit(FALSE, TRUE) /\ UNCHANGED <<mapped, override, aborts, fixes>>
         [] mapped /\ author = "override" ->      \* local override: offer to remove it
              \E yes \in BOOLEAN :
                IF yes
                  THEN /\ override' = FALSE /\ fixes' = fixes + 1
                       /\ IF FixOverrideThenContinue
                            THEN Commit(FALSE, FALSE) /\ UNCHANGED aborts
                            ELSE Abort /\ UNCHANGED commits   \* "Re-run your commit."
                       /\ UNCHANGED mapped
                  ELSE Commit(TRUE, FALSE) /\ UNCHANGED <<mapped, override, aborts, fixes>>
         [] mapped ->                             \* any other mismatch: warn, commit as-is
              Commit(FALSE, TRUE) /\ UNCHANGED <<mapped, override, aborts, fixes>>
         [] ~interactive ->                       \* unmapped, IDE: warn
              Commit(FALSE, TRUE) /\ UNCHANGED <<mapped, override, aborts, fixes>>
         [] OTHER ->                              \* unmapped: "Is this right?"
              \E right \in BOOLEAN :
                IF right
                  THEN Commit(TRUE, FALSE) /\ UNCHANGED <<mapped, override, aborts, fixes>>
                  ELSE /\ mapped' = TRUE /\ fixes' = fixes + 1   \* pick a profile, map the repo
                       /\ Abort /\ UNCHANGED <<commits, override>>
    /\ UNCHANGED <<genOK, interactive, explicit, author, attempts, budget>>

Done == pc = "idle" /\ attempts = MaxAttempts

Next == Tamper \/ AddOverride \/ Begin \/ Heal \/ Check \/ (Done /\ UNCHANGED vars)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
\* No commit is ever made with a wrong identity silently: it carries the
\* mapped profile, or the user chose otherwise (--author, or "no, keep it"),
\* or gids couldn't ask and warned (no terminal).
NoSilentWrongIdentity ==
    \A c \in commits :
        \/ c.author \in {"profile", "explicit"}
        \/ c.accepted
        \/ c.warned

\* Stronger, in a terminal (where gids can ask): a mapped repo never gets a
\* commit with a wrong identity unless the user chose it (--author, or
\* "no, keep the override"). Warnings alone aren't consent.
ConsentInTerminal ==
    \A c \in commits :
        (c.mapped /\ c.interactive) =>
            (c.author \in {"profile", "explicit"} \/ c.accepted)

\* gids never stops a commit without fixing something: every abort repaired
\* a real problem, so re-running converges (no endless abort loop).
EveryAbortFixesSomething == aborts <= fixes
=============================================================================
