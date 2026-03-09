package cmd

import (
	"bufio"
	"fmt"
	"os"

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

			p := cfg.LookupProfile(name)
			if p == nil {
				return fmt.Errorf("profile %q not found", name)
			}
			if err := p.Validate(); err != nil {
				return fmt.Errorf("profile %q is incomplete: %w", name, err)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}

			client := git.New(cwd)
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

			// If this directory is not yet governed by a rule, offer to save one.
			_, alreadyMapped := config.MatchRule(cfg.Rules, cwd)
			if !alreadyMapped {
				displayDir := tildify(cwd)
				r := bufio.NewReader(cmd.InOrStdin())
				save, err := confirmPrompt(r, cmd.OutOrStdout(),
					fmt.Sprintf("Always use %q for %s?", name, displayDir), true)
				if err != nil {
					return fmt.Errorf("reading response: %w", err)
				}
				if save {
					cfg.AddRule(displayDir, name)
					if err := config.Save(cfg, cfgPath); err != nil {
						return fmt.Errorf("saving rule: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Rule saved: %s -> %s\n", displayDir, name)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "",
		"path to config file (default: $UserConfigDir/gids/config.yaml)")
	return cmd
}
