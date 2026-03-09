package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
)

func newStatusCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show the active profile and how it was applied",
		Example: "  gids status",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}

			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			client := git.New(cwd)
			isRepo, err := client.IsRepo()
			if err != nil {
				return fmt.Errorf("checking git repo: %w", err)
			}
			if !isRepo {
				fmt.Fprintln(cmd.OutOrStdout(), "Not in a git repository.")
				return nil
			}

			// ConfigGet reads only the local .git/config scope (--local).
			// A global ~/.gitconfig identity will appear as "(not set)" here,
			// which is intentional: gids manages local repo identities only.
			gitName, err := client.ConfigGet("user.name")
			if err != nil {
				return fmt.Errorf("reading user.name: %w", err)
			}
			gitEmail, err := client.ConfigGet("user.email")
			if err != nil {
				return fmt.Errorf("reading user.email: %w", err)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

			if gitName == "" || gitEmail == "" {
				fmt.Fprintln(w, "Identity:\t(not set)")
				fmt.Fprintln(w, "Profile:\t(not set)")
			} else {
				fmt.Fprintf(w, "Identity:\t%s <%s>\n", gitName, gitEmail)
				p := cfg.LookupProfileByIdentity(gitName, gitEmail)
				profileName := "(unrecognized)"
				if p != nil {
					profileName = p.Name
				}
				fmt.Fprintf(w, "Profile:\t%s\n", profileName)
			}

			glob, _, ok := config.FindMatchingRule(cfg.Rules, cwd)
			if ok {
				fmt.Fprintf(w, "Source:\tRule (%s)\n", glob)
			} else {
				fmt.Fprintln(w, "Source:\tManual")
			}

			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "",
		"path to config file (default: $UserConfigDir/gids/config.yaml)")
	return cmd
}
