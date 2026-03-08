package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
)

func newUseCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "use <profile>",
		Short: "Apply a profile to the current git repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			p, _ := cfg.FindProfile(name)
			if p == nil {
				return fmt.Errorf("profile %q not found", name)
			}
			if err := p.Validate(); err != nil {
				return fmt.Errorf("profile %q is incomplete: %w", name, err)
			}

			client := git.New(".")
			ok, err := client.IsRepo()
			if err != nil {
				return fmt.Errorf("checking git repo: %w", err)
			}
			if !ok {
				return fmt.Errorf("not a git repository")
			}

			if err := git.Apply(client, *p); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Applied profile %q (%s <%s>).\n",
				p.Name, p.GitName, p.GitEmail)
			return nil
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "",
		"path to config file (default: $UserConfigDir/gids/config.yaml)")
	return cmd
}
