---------------------------- MODULE GidsFiles ----------------------------
(***************************************************************************)
(* gids' file protocol (SPEC §7.2, §11): config.yaml, the generated files  *)
(* and the ~/.gitconfig block, changed by concurrent gids processes that   *)
(* can crash (kill -9) at any step, while the user may tamper with the     *)
(* generated files.                                                        *)
(*                                                                         *)
(* One atomic step = the code between two testhook.Point calls in the real *)
(* code (VER-2). TestTracesFollowTheModeledProtocol checks that the real   *)
(* traces have the step order modeled here.                                *)
(*                                                                         *)
(* Abstractions (audited in verify/README.md): config.yaml is the set of   *)
(* profile ids saved in it; "the generated files" are one value, the       *)
(* config they were rendered from (Absent when there are none, Mixed while *)
(* a multi-file sync is half done, Tampered after the user edits them).    *)
(***************************************************************************)
EXTENDS FiniteSets, Naturals

CONSTANTS Procs,        \* concurrent gids processes
          MaxOps,       \* commands each process runs
          UseLock,      \* FALSE: negative control (no cross-process lock)
          HealLoadsInLock,  \* FALSE: negative control (heal from a config loaded earlier)
          InstallBlockFirst, \* TRUE: negative control (activate before files exist)
          UninstallFilesFirst, \* TRUE: negative control (delete files before deactivating)
          BrokenLock    \* TRUE: negative control (taking the lock doesn't check the holder)

\* Non-config values of the generated files, as sets so TLC can compare
\* them with configs (which are sets of profile ids).
Absent == {"absent"}
Mixed == {"mixed"}
Tampered == {"tampered"}
None == "none"

\* The generated files that should exist for config c and installed b.
Desired(c, b) == IF b THEN c ELSE Absent

VARIABLES
    cfg,        \* profile ids saved in config.yaml (renames are atomic)
    gen,        \* the generated files: a config value, Absent, Mixed or Tampered
    block,      \* is the ~/.gitconfig block present (gids installed)?
    temps,      \* processes whose temp file for config.yaml exists
    holder,     \* process holding the lock, or None (released by the OS on crash)
    pc, cmd, local, ops,
    committed,  \* ids whose `profile add` reported success (history)
    stale,      \* a sync wrote files from a config that's no longer current (history)
    stolen,     \* a process's in-flight temp file was deleted by another (history)
    tampers     \* user tampering left (adversary budget)

vars == <<cfg, gen, block, temps, holder, pc, cmd, local, ops, committed, stale, stolen, tampers>>

Commands == {"add", "heal", "install", "uninstall"}

Init ==
    /\ cfg = {}
    /\ gen = {}            \* installed, rendered from the empty config
    /\ block = TRUE
    /\ temps = {}
    /\ holder = None
    /\ pc = [p \in Procs |-> "idle"]
    /\ cmd = [p \in Procs |-> "none"]
    /\ local = [p \in Procs |-> {}]
    /\ ops = [p \in Procs |-> 0]
    /\ committed = {}
    /\ stale = FALSE
    /\ stolen = FALSE
    /\ tampers = 1

Free(p) == holder \in {None, p}

\* --- starting and finishing commands ---------------------------------------
Start(p) ==
    /\ pc[p] = "idle" /\ ops[p] < MaxOps
    /\ \E c \in Commands :
         /\ cmd' = [cmd EXCEPT ![p] = c]
         /\ pc' = [pc EXCEPT ![p] =
                    IF c = "heal" /\ ~HealLoadsInLock THEN "preload" ELSE "lock"]
    /\ ops' = [ops EXCEPT ![p] = @ + 1]
    /\ UNCHANGED <<cfg, gen, block, temps, holder, local, committed, stale, stolen, tampers>>

\* Negative control: heal loads config.yaml before taking the lock.
PreLoad(p) ==
    /\ pc[p] = "preload"
    /\ local' = [local EXCEPT ![p] = cfg]
    /\ pc' = [pc EXCEPT ![p] = "lock"]
    /\ UNCHANGED <<cfg, gen, block, temps, holder, cmd, ops, committed, stale, stolen, tampers>>

Lock(p) ==
    /\ pc[p] = "lock"
    /\ IF UseLock THEN (BrokenLock \/ holder = None) /\ holder' = p ELSE UNCHANGED holder
    /\ pc' = [pc EXCEPT ![p] = IF cmd[p] = "heal" /\ ~HealLoadsInLock THEN "sync1" ELSE "load"]
    /\ UNCHANGED <<cfg, gen, block, temps, cmd, local, ops, committed, stale, stolen, tampers>>

Load(p) ==
    /\ pc[p] = "load"
    /\ local' = [local EXCEPT ![p] = cfg]
    /\ pc' = [pc EXCEPT ![p] =
          CASE cmd[p] = "add"       -> "tmp"
            [] cmd[p] = "heal"      -> "sync1"
            [] cmd[p] = "install"   -> IF InstallBlockFirst THEN "activate" ELSE "sync1"
            [] cmd[p] = "uninstall" -> IF UninstallFilesFirst THEN "sync1" ELSE "deactivate"]
    /\ UNCHANGED <<cfg, gen, block, temps, holder, cmd, ops, committed, stale, stolen, tampers>>

\* --- `profile add`: write config.yaml via temp file + rename ---------------
Tmp(p) ==
    /\ pc[p] = "tmp"
    /\ temps' = temps \cup {p}
    /\ pc' = [pc EXCEPT ![p] = "rename"]
    /\ UNCHANGED <<cfg, gen, block, holder, cmd, local, ops, committed, stale, stolen, tampers>>

Rename(p) ==
    /\ pc[p] = "rename"
    /\ IF p \in temps
         THEN /\ cfg' = local[p] \cup {p}          \* p's profile id is p
              /\ local' = [local EXCEPT ![p] = local[p] \cup {p}]
              /\ temps' = temps \ {p}
              /\ committed' = committed \cup {p}       \* saved: the add has taken effect
              /\ pc' = [pc EXCEPT ![p] = "sync1"]
              /\ UNCHANGED stolen
         ELSE \* another process deleted our temp file: the rename fails
              /\ stolen' = TRUE
              /\ pc' = [pc EXCEPT ![p] = "unlock"]
              /\ UNCHANGED <<cfg, local, temps, committed>>
    /\ UNCHANGED <<gen, block, holder, cmd, ops, stale, tampers>>

\* --- sync: make the generated files match (two steps: it's many files) -----
\* The value it writes for `install` is for installed=TRUE even before the
\* block exists; `uninstall` writes Absent.
Target(p) ==
    CASE cmd[p] = "install"   -> local[p]
      [] cmd[p] = "uninstall" -> Absent
      [] OTHER                -> Desired(local[p], block)

Sync1(p) ==   \* first file written / removed, plus leftover temp files deleted
    /\ pc[p] = "sync1"
    /\ gen' = Mixed
    /\ temps' = {}                                  \* stray temp files are removed
    /\ pc' = [pc EXCEPT ![p] = "sync2"]
    /\ UNCHANGED <<cfg, block, holder, cmd, local, ops, committed, stale, stolen, tampers>>

Sync2(p) ==   \* last file: the generated set is now what p rendered
    /\ pc[p] = "sync2"
    /\ gen' = Target(p)
    \* history: was p's view current? (install/uninstall compare to their own goal)
    /\ stale' = (stale \/ (cmd[p] \in {"add", "heal"} /\ Target(p) # Desired(cfg, block)))
    /\ pc' = [pc EXCEPT ![p] =
          CASE cmd[p] = "install"   -> IF InstallBlockFirst THEN "unlock" ELSE "activate"
            [] cmd[p] = "uninstall" -> IF UninstallFilesFirst THEN "deactivate" ELSE "unlock"
            [] OTHER                -> "unlock"]
    /\ UNCHANGED <<cfg, block, temps, holder, cmd, local, ops, committed, stolen, tampers>>

Activate(p) ==    \* one atomic write of ~/.gitconfig adds the block
    /\ pc[p] = "activate"
    /\ block' = TRUE
    /\ pc' = [pc EXCEPT ![p] = IF InstallBlockFirst THEN "sync1" ELSE "unlock"]
    /\ UNCHANGED <<cfg, gen, temps, holder, cmd, local, ops, committed, stale, stolen, tampers>>

Deactivate(p) ==  \* one atomic write of ~/.gitconfig removes the block
    /\ pc[p] = "deactivate"
    /\ block' = FALSE
    /\ pc' = [pc EXCEPT ![p] = IF UninstallFilesFirst THEN "unlock" ELSE "sync1"]
    /\ UNCHANGED <<cfg, gen, temps, holder, cmd, local, ops, committed, stale, stolen, tampers>>

Unlock(p) ==
    /\ pc[p] = "unlock"
    /\ holder' = IF holder = p THEN None ELSE holder
    /\ pc' = [pc EXCEPT ![p] = "idle"]
    /\ UNCHANGED <<cfg, gen, block, temps, cmd, local, ops, committed, stale, stolen, tampers>>

\* --- the environment ---------------------------------------------------------
Crash(p) ==   \* kill -9 at any step: the OS releases the lock; temp files stay
    /\ pc[p] \notin {"idle", "crashed"}
    /\ pc' = [pc EXCEPT ![p] = "crashed"]
    /\ holder' = IF holder = p THEN None ELSE holder
    /\ UNCHANGED <<cfg, gen, block, temps, cmd, local, ops, committed, stale, stolen, tampers>>

Restart(p) == \* the user runs gids again in that terminal
    /\ pc[p] = "crashed"
    /\ pc' = [pc EXCEPT ![p] = "idle"]
    /\ UNCHANGED <<cfg, gen, block, temps, holder, cmd, local, ops, committed, stale, stolen, tampers>>

Tamper ==     \* the user edits a generated file
    /\ tampers > 0
    /\ gen' = Tampered
    /\ tampers' = tampers - 1
    /\ UNCHANGED <<cfg, block, temps, holder, pc, cmd, local, ops, committed, stale, stolen>>

Step(p) == Start(p) \/ PreLoad(p) \/ Lock(p) \/ Load(p) \/ Tmp(p) \/ Rename(p)
           \/ Sync1(p) \/ Sync2(p) \/ Activate(p) \/ Deactivate(p) \/ Unlock(p)
           \/ Crash(p) \/ Restart(p)

Done == \A p \in Procs : pc[p] = "idle" /\ ops[p] = MaxOps

Next == (\E p \in Procs : Step(p)) \/ Tamper \/ (Done /\ UNCHANGED vars)

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
\* Invariants

TypeOK ==
    /\ cfg \subseteq Procs
    /\ gen \in (SUBSET Procs) \cup {Absent, Mixed, Tampered}
    /\ block \in BOOLEAN
    /\ holder \in Procs \cup {None}

\* Lock: at most one process is ever inside the critical section.
MutualExclusion ==
    UseLock => Cardinality({p \in Procs : pc[p] \in {"load", "tmp", "rename", "sync1", "sync2", "activate", "deactivate", "unlock"}}) <= 1

\* No `profile add` that reported success is ever undone by another (no lost update).
NoLostUpdate == committed \subseteq cfg

\* No command fails because another deleted its in-flight temp file.
NoStolenTemp == ~stolen

\* Every completed sync wrote what the *current* config implies: a heal can't
\* leave files rendered from an outdated config (I2 after every command).
NoStaleSync == ~stale

\* Git never sees the block while the files it includes are absent (I4 as
\* git sees it: the hooks dir and include target exist whenever the block does).
ActivationSafe == block => gen # Absent

=============================================================================
