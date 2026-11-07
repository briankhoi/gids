package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newHookExecCmd is what gids' hook shims call. It never fails a commit
// because of its own problems (fail open, SPEC §8). No hook does anything
// yet: the shims only chain to the repo's own hooks.
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
			return nil
		},
	}
}
