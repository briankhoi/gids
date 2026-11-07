package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
)

func newRuleCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rule",
		Short: "Map directories to profiles",
		Long: `A rule maps a directory to a profile. It applies to every repo inside that
directory, at any depth. When rules overlap, the deepest directory wins.`,
	}
	cmd.AddCommand(newRuleAddCmd(c), newRuleListCmd(c), newRuleRemoveCmd(c))
	return cmd
}

// normalizeDir turns a user-typed directory into a rule key.
func (c *cli) normalizeDir(dir string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return config.NormalizeRule(dir, cwd, c.env.Paths.Home)
}

// setRule saves dir → profile (profile matched case-insensitively) and reports.
func (c *cli) setRule(dir, profile string) error {
	var name, prev string
	_, err := c.env.Mutate(func(cfg *config.Config) error {
		p := cfg.ProfileFold(profile)
		if p == nil {
			return fmt.Errorf("profile %q not found (see: gids profile list)", profile)
		}
		name, prev = p.Name, cfg.Rules[dir]
		cfg.SetRule(dir, p.Name)
		return nil
	})
	if err != nil {
		return err
	}
	switch prev {
	case "":
		c.ui.Success("Rule saved: %s → %s", dir, name)
	case name:
		c.ui.Success("Rule unchanged: %s → %s", dir, name)
	default:
		c.ui.Success("Rule updated: %s → %s (was %s)", dir, name, prev)
	}
	c.warnNotInstalled()
	return nil
}

func newRuleAddCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:     "add <dir> <profile>",
		Short:   "Use <profile> for every repo inside <dir>",
		Example: "  gids rule add ~/work Work\n  gids rule add . Personal",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := c.normalizeDir(args[0])
			if err != nil {
				return err
			}
			return c.setRule(dir, args[1])
		},
	}
}

func newRuleListCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List rules (later lines win where they overlap)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := c.load()
			if err != nil {
				return err
			}
			if len(cfg.Rules) == 0 {
				c.ui.Printf("No rules yet. Add one: gids rule add <dir> <profile>\n")
				return nil
			}
			tw := tabwriter.NewWriter(c.ui.Out(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "DIRECTORY\tPROFILE")
			for _, dir := range config.SortedRules(cfg.Rules, c.env.Paths.Home) {
				fmt.Fprintf(tw, "%s\t%s\n", dir, cfg.Rules[dir])
			}
			return tw.Flush()
		},
	}
}

func newRuleRemoveCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <dir>",
		Aliases: []string{"rm"},
		Short:   "Remove the rule for <dir>",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := c.normalizeDir(args[0])
			if err != nil {
				return err
			}
			var was string
			if _, err := c.env.Mutate(func(cfg *config.Config) error {
				p, ok := cfg.Rules[dir]
				if !ok {
					return fmt.Errorf("no rule for %s (see: gids rule list)", dir)
				}
				was = p
				delete(cfg.Rules, dir)
				return nil
			}); err != nil {
				return err
			}
			c.ui.Success("Removed rule: %s → %s", dir, was)
			return nil
		},
	}
}

// warnNotInstalled reminds the user that rules do nothing until installed.
func (c *cli) warnNotInstalled() {
	if ok, err := c.env.Installed(); err == nil && !ok {
		c.ui.Warn("gids isn't installed yet, so rules don't apply. Run: gids install")
	}
}
