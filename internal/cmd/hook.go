package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/ui"
)

// newHookExecCmd is what gids' hook shims call. It never fails a commit
// because of its own problems (fail open, SPEC §8); it exits non-zero only to
// deliberately stop a commit (identity fixed → re-run, or Ctrl-C).
func newHookExecCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:    "hook-exec <hook> [args...]",
		Short:  "Run a gids git hook (called by the hooks gids installs)",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if os.Getenv("GIDS_SKIP_HOOKS") == "1" {
				return nil
			}
			defer func() { // a gids bug must never block a commit
				if r := recover(); r != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "gids: internal error in %s hook (%v); skipping\n", args[0], r)
					err = nil
				}
			}()
			cwd, err := os.Getwd()
			if err != nil {
				return nil
			}
			h := &hookRun{c: c, g: git.Client{Dir: cwd}, stderr: cmd.ErrOrStderr()}
			var code int
			switch args[0] {
			case "pre-commit":
				code = h.preCommit()
			case "commit-msg":
				if len(args) > 1 {
					h.commitMsg(args[1])
				}
			case "post-commit":
				h.postCommit()
			}
			if code != 0 {
				return exitError{code}
			}
			return nil
		},
	}
}

type hookRun struct {
	c      *cli
	g      git.Client
	stderr io.Writer
}

// note prints a one-line message from a hook to the user's terminal (hook
// stderr is shown by git, also in most IDEs' git logs).
func (h *hookRun) note(format string, a ...any) {
	fmt.Fprintf(h.stderr, "gids: "+format+"\n", a...)
}

// openTTY opens the user's terminal for prompting from inside a hook, whose
// stdin git sets to /dev/null. GIDS_TTY substitutes a file of scripted
// answers (tests); then output goes to stderr.
func openTTY() (in io.Reader, out io.Writer, closer func(), ok bool) {
	if p := os.Getenv("GIDS_TTY"); p != "" {
		f, err := os.Open(p)
		if err != nil {
			return nil, nil, nil, false
		}
		return f, os.Stderr, func() { f.Close() }, true
	}
	if runtime.GOOS == "windows" {
		in, err := os.Open("CONIN$")
		if err != nil {
			return nil, nil, nil, false
		}
		out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			in.Close()
			return nil, nil, nil, false
		}
		return in, out, func() { in.Close(); out.Close() }, true
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, false
	}
	return f, f, func() { f.Close() }, true
}

// commitIdent is who this commit will be authored as. git exports it to the
// pre-commit hook; `git var` is the fallback.
func (h *hookRun) commitIdent() (name, email string) {
	name, email = os.Getenv("GIT_AUTHOR_NAME"), os.Getenv("GIT_AUTHOR_EMAIL")
	if email != "" {
		return name, email
	}
	if out, err := h.g.Run("var", "GIT_AUTHOR_IDENT"); err == nil {
		lt, gt := strings.IndexByte(out, '<'), strings.IndexByte(out, '>')
		if lt > 0 && gt > lt {
			return strings.TrimSpace(out[:lt]), out[lt+1 : gt]
		}
	}
	return name, email
}

// preCommit is the commit guard (SPEC §8). Returns the hook's exit code.
func (h *hookRun) preCommit() int {
	env := h.c.env
	cfg, _, identityChanged, err := env.Heal()
	if cfg == nil {
		h.note("%v; skipping the identity check", err)
		return 0
	}
	if err != nil {
		h.note("couldn't repair generated config: %v", err)
	} else if identityChanged {
		h.note("repaired its generated git config, which can change this commit's identity. Re-run your commit.")
		return 1
	}
	repo, err := h.g.Repo()
	if err != nil {
		return 0
	}
	name, email := h.commitIdent()
	mapped, _, _ := h.g.ConfigGet("gids.profile")
	dir := env.Display(repo.RuleDir())

	in, out, closeTTY, interactive := openTTY()
	if interactive {
		defer closeTTY()
	} else {
		in, out = strings.NewReader(""), h.stderr
	}
	u := ui.New(h.c.ui.Context(), in, out)

	if p := cfg.Profile(mapped); p != nil {
		if strings.EqualFold(email, p.GitEmail) && name == p.GitName {
			return 0
		}
		return h.mismatch(u, interactive, p, name, email)
	}

	// Unmapped repo.
	if !interactive {
		h.note("no rule for %s; committing as %s <%s>", dir, name, email)
		return 0
	}
	code, err := h.unmapped(u, cfg, repo, name, email)
	return hookExit(code, err)
}

// hookExit maps prompt outcomes: Ctrl-C stops the commit (130); Ctrl-D/Esc
// and other errors let it proceed unchanged.
func hookExit(code int, err error) int {
	switch {
	case errors.Is(err, ui.ErrInterrupted):
		return 130
	case err != nil:
		return 0
	}
	return code
}

func (h *hookRun) mismatch(u *ui.UI, interactive bool, p *config.Profile, name, email string) int {
	// Which setting is wrong, and where does the wrong value come from?
	key, got := "user.email", email
	if strings.EqualFold(email, p.GitEmail) {
		key, got = "user.name", name
	}
	src, _, _ := h.g.ConfigSource(key)
	source, local := h.c.showOrigin(src.Origin), src.Scope == "local"
	if src.Value != got { // the config agrees with the profile: --author or GIT_AUTHOR_* env
		source, local = "--author or GIT_AUTHOR_* environment variables", false
	}
	msg := fmt.Sprintf("This commit is authored as %s <%s>, but this repo is mapped to %s (%s). Source: %s", name, email, p.Name, p.Ident(), source)
	if !interactive {
		h.note("%s", msg)
		if local {
			h.note("fix it with: gids use %s", p.Name)
		}
		return 0
	}
	u.Warn("%s", msg)
	if !local {
		u.Hint("gids only fixes overrides in .git/config; committing as-is.")
		return 0
	}
	ok, err := u.Confirm("Remove the local override and re-run?", true)
	if err != nil || !ok {
		return hookExit(0, err)
	}
	for _, k := range identityKeys {
		if err := h.g.ConfigUnsetLocal(k); err != nil {
			u.Fail("couldn't remove %s: %v", k, err)
			return 0
		}
	}
	u.Success("Fixed: this repo now uses %s. Re-run your commit.", p.Name)
	return 1
}

// unmapped handles a repo no rule covers. Returns 1 when the identity was
// changed (so the commit must be re-run as the new identity).
func (h *hookRun) unmapped(u *ui.UI, cfg *config.Config, repo git.Repo, name, email string) (int, error) {
	env := h.c.env
	dir := env.Display(repo.RuleDir())
	u.Printf("Committing as %s <%s> in %s (no gids rule).\n", name, email, dir)
	right, err := u.Confirm("Is this right?", true)
	if err != nil {
		return 0, err
	}
	ruleKey, err := config.NormalizeRule(repo.RuleDir(), repo.RuleDir(), env.Paths.Home)
	if err != nil {
		return 0, err
	}
	saveRule := func(profile string) error {
		_, err := env.Mutate(func(cfg *config.Config) error {
			if cfg.Profile(profile) == nil {
				return fmt.Errorf("profile %q disappeared", profile)
			}
			cfg.SetRule(ruleKey, profile)
			return nil
		})
		if err == nil {
			u.Success("Rule saved: %s → %s", ruleKey, profile)
		}
		return err
	}

	if right {
		if p := cfg.ProfileByIdentity(name, email); p != nil {
			ok, err := u.Confirm(fmt.Sprintf("Always use %s for %s?", p.Name, dir), true)
			if err != nil || !ok {
				return 0, err
			}
			return 0, saveRule(p.Name)
		}
		pname, err := u.Input("Save as a new profile? Name (Enter to skip)", "", func(s string) error {
			if s == "" {
				return nil
			}
			if err := config.ValidateProfileName(s); err != nil {
				return err
			}
			if cfg.ProfileFold(s) != nil {
				return fmt.Errorf("a profile named %q already exists", s)
			}
			return nil
		})
		if err != nil || pname == "" {
			return 0, err
		}
		p := config.Profile{Name: pname, GitName: name, GitEmail: email}
		if err := p.Validate(); err != nil {
			u.Fail("can't save this identity as a profile: %v", err)
			return 0, nil
		}
		if _, err := env.Mutate(func(cfg *config.Config) error { return cfg.AddProfile(p) }); err != nil {
			return 0, err
		}
		ok, err := u.Confirm(fmt.Sprintf("Always use %s for %s?", pname, dir), true)
		if err != nil || !ok {
			return 0, err
		}
		return 0, saveRule(pname)
	}

	// Wrong identity: choose or create the right profile, map the repo,
	// then stop the commit so it's re-run as that identity.
	const newRow = "+ New profile"
	items := []ui.Item{}
	for _, p := range cfg.Profiles {
		items = append(items, ui.Item{Label: p.Name, Detail: p.Ident()})
	}
	choice := len(items)
	if len(items) > 0 {
		items = append(items, ui.Item{Label: newRow, Detail: "create one now"})
		if choice, err = u.Select("Commit as which profile?", items); err != nil {
			return 0, err
		}
	}
	var pname string
	if choice < len(cfg.Profiles) {
		pname = cfg.Profiles[choice].Name
	} else {
		if len(cfg.Profiles) == 0 {
			u.Printf("No profiles yet. Let's make one.\n")
		}
		p, err := h.c.withUI(u).profileForm("New profile", config.Profile{GitName: name, GitEmail: email},
			func(s string) bool { return cfg.ProfileFold(s) != nil })
		if err != nil {
			return 0, err
		}
		if _, err := env.Mutate(func(cfg *config.Config) error { return cfg.AddProfile(p) }); err != nil {
			return 0, err
		}
		pname = p.Name
	}
	if err := saveRule(pname); err != nil {
		return 0, err
	}
	u.Success("This repo now uses %s. Re-run your commit to commit as %s.", pname, pname)
	return 1, nil
}

// withUI returns a copy of c that talks through u (the terminal, in hooks).
func (c *cli) withUI(u *ui.UI) *cli { cp := *c; cp.ui = u; return &cp }

// commitMsg appends the session's Co-authored-by trailers (SPEC §9.1). It
// leaves an empty message alone so git still aborts it (PROGRESS.md D1).
func (h *hookRun) commitMsg(file string) {
	authors, err := h.g.ConfigGetAll("gids.coauthor")
	if err != nil || len(authors) == 0 {
		return
	}
	msg, err := os.ReadFile(file)
	if err != nil {
		return
	}
	stripped, err := h.g.RunInput(string(msg), "stripspace", "--strip-comments")
	if err != nil || strings.TrimSpace(stripped) == "" {
		return
	}
	args := []string{"interpret-trailers", "--in-place", "--if-exists", "addIfDifferent"}
	for _, a := range authors {
		if validCoauthor(a) {
			args = append(args, "--trailer", "Co-authored-by: "+a)
		}
	}
	if _, err := h.g.Run(append(args, file)...); err != nil {
		h.note("couldn't add co-authors: %v", err)
	}
}

// postCommit ends a --once pairing session after a successful commit.
func (h *hookRun) postCommit() {
	once, _, _ := h.g.ConfigGet("gids.coauthorOnce", "--local", "--type=bool")
	if once != "true" {
		return
	}
	if err := clearSession(h.g); err != nil {
		h.note("couldn't clear one-time co-authors: %v", err)
		return
	}
	h.note("one-time co-authors cleared")
}
