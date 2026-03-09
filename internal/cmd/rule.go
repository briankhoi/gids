package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
)

func newRuleCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "rule",
		Short: "Manage directory-to-profile rules",
		Long: `Manage directory-to-profile rules.

Rules map directory glob patterns to profiles so that 'gids check' (called
by the shell hook) can apply the right profile automatically when you cd.`,
	}

	cmd.PersistentFlags().StringVar(&cfgPath, "config", "",
		"path to config file (default: $UserConfigDir/gids/config.yaml)")

	cmd.AddCommand(newRuleListCmd(&cfgPath))
	cmd.AddCommand(newRuleAddCmd(&cfgPath))
	cmd.AddCommand(newRuleRemoveCmd(&cfgPath))

	return cmd
}

func newRuleListCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all directory-to-profile rules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(cfg.Rules) == 0 {
				fmt.Fprintln(out, "No rules configured. Run 'gids rule add' to create one.")
				return nil
			}

			// Sort globs for stable output.
			globs := make([]string, 0, len(cfg.Rules))
			for g := range cfg.Rules {
				globs = append(globs, g)
			}
			sort.Strings(globs)

			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "GLOB\tPROFILE")
			for _, g := range globs {
				fmt.Fprintf(tw, "%s\t%s\n", g, cfg.Rules[g])
			}
			return tw.Flush()
		},
	}
}

func newRuleAddCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "add <glob> <profile>",
		Short: "Add a directory-to-profile rule",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			glob, profileName := args[0], args[1]

			// Validate glob syntax before touching the config.
			// filepath.Match returns ErrBadPattern for malformed patterns
			// regardless of the path argument, so "" suffices here.
			if _, err := filepath.Match(glob, ""); err != nil {
				return fmt.Errorf("invalid glob pattern %q: %w", glob, err)
			}

			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if cfg.LookupProfile(profileName) == nil {
				return fmt.Errorf("profile %q not found", profileName)
			}

			cfg.AddRule(glob, profileName)

			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added rule: %s -> %s\n", glob, profileName)
			return nil
		},
	}
}

func newRuleRemoveCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove [<glob>]",
		Short: "Remove a directory-to-profile rule",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			var glob, profileName string

			if len(args) == 1 {
				// Explicit glob provided — look it up directly.
				glob = args[0]
				p, exists := cfg.Rules[glob]
				if !exists {
					return fmt.Errorf("no rule found for %q", glob)
				}
				profileName = p
			} else {
				// No arg: find the rule matching the current directory.
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting current directory: %w", err)
				}
				g, p, ok := config.FindMatchingRule(cfg.Rules, cwd)
				if !ok {
					return fmt.Errorf("no rule found for current directory %q", tildify(cwd))
				}
				glob = g
				profileName = p
			}

			cfg.RemoveRule(glob)

			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed rule: %s -> %s\n", glob, profileName)
			return nil
		},
	}
}
