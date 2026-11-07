// Package cmd is gids' command-line interface.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"gids/internal/app"
	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/ui"
	"gids/internal/version"
)

// exitError ends the process with a specific code and no extra message.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

// cli is what every command gets: the environment and a UI on its streams.
type cli struct {
	env *app.Env
	ui  *ui.UI
}

// NewRoot builds the command tree.
func NewRoot() *cobra.Command {
	c := &cli{}
	root := &cobra.Command{
		Use:   "gids",
		Short: "Git Identity Swap: the right git identity in every directory",
		Long: `gids manages your git identities (name, email, SSH key, signing key) and maps
directories to them. Git itself applies the right identity everywhere (terminal,
IDE, scripts) through includeIf; gids writes that config for you and guards your
commits against the wrong identity.

Get started:
  gids install             # once
  gids profile add         # for each identity
  gids rule add ~/work Work
  gids use Personal        # map the current repo`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Annotations["heal"] == "skip" {
				return nil
			}
			env, err := app.NewEnv()
			if err != nil {
				if cmd.Name() == "hook-exec" { // fail open: never block a commit
					fmt.Fprintf(cmd.ErrOrStderr(), "gids: %v; skipping\n", err)
					return exitError{0}
				}
				return err
			}
			c.env = env
			c.ui = ui.New(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
			if cmd.Annotations["heal"] == "no" {
				return nil
			}
			// Self-heal before anything else (SPEC §7.2).
			cfg, err := env.Load()
			if err != nil {
				return err
			}
			_, _, err = env.Heal(cfg)
			return err
		},
	}
	// Per-command setup: "" = load env and self-heal (default); "no" = load
	// env without healing (install, uninstall, hooks manage that themselves);
	// "skip" = needs nothing (version).
	add := func(cmd *cobra.Command, mode string) {
		if mode != "" {
			cmd.Annotations = map[string]string{"heal": mode}
		}
		root.AddCommand(cmd)
	}
	add(newVersionCmd(), "skip")
	add(newInstallCmd(c), "no")
	add(newUninstallCmd(c), "no")
	add(newHookExecCmd(c), "no")
	add(newRuleCmd(c), "")
	for _, sub := range root.Commands() {
		inheritAnnotations(sub)
	}
	return root
}

// inheritAnnotations copies a parent's "heal" annotation to subcommands.
func inheritAnnotations(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		if sub.Annotations == nil && cmd.Annotations != nil {
			sub.Annotations = cmd.Annotations
		}
		inheritAnnotations(sub)
	}
}

// Execute runs gids with the process's args and streams, and returns the exit
// code: 0 ok, 1 error or Ctrl-D/Esc, 130 Ctrl-C.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, NewRoot(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

func run(ctx context.Context, root *cobra.Command, args []string, in io.Reader, out, errOut io.Writer) int {
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	err := root.ExecuteContext(ctx)
	var ee exitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		return ee.code
	case errors.Is(err, ui.ErrInterrupted) || ctx.Err() != nil:
		fmt.Fprintln(errOut, "Cancelled.")
		return 130
	case errors.Is(err, ui.ErrSkipped):
		fmt.Fprintln(errOut, "Cancelled.")
		return 1
	default:
		u := ui.New(ctx, in, errOut)
		u.Fail("%v", err)
		return 1
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the gids version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "gids %s\n", version.Get())
		},
	}
}

// load reads config.yaml (used for read-only lookups before prompting; any
// change goes through env.Mutate, which reloads).
func (c *cli) load() (*config.Config, error) { return c.env.Load() }

// repo returns a git client for the current directory and its repo.
func (c *cli) repo() (git.Client, git.Repo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return git.Client{}, git.Repo{}, err
	}
	g := git.Client{Dir: cwd}
	r, err := g.Repo()
	return g, r, err
}
