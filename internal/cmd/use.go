package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/ui"
)

// pickProfile shows the profile picker.
func (c *cli) pickProfile(title string, cfg *config.Config) (string, error) {
	if len(cfg.Profiles) == 0 {
		return "", fmt.Errorf("no profiles yet; add one with: gids profile add")
	}
	items := make([]ui.Item, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		items[i] = ui.Item{Label: p.Name, Detail: p.Ident()}
	}
	i, err := c.ui.Select(title, items)
	if err != nil {
		return "", err
	}
	return cfg.Profiles[i].Name, nil
}

func newUseCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:               "use [<profile>]",
		Short:             "Always use <profile> in the current repo",
		Long:              "Saves a rule mapping the current repo to <profile> (no argument: pick one).",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: c.completeProfiles,
		RunE: func(cmd *cobra.Command, args []string) error {
			g, r, err := c.repo()
			if err != nil {
				return err
			}
			cfg, err := c.load()
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			} else if name, err = c.pickProfile("Use which profile here?", cfg); err != nil {
				return err
			}
			dir, err := config.NormalizeRule(r.RuleDir(), r.RuleDir(), c.env.Paths.Home)
			if err != nil {
				return err
			}
			if err := c.setRule(dir, name); err != nil {
				return err
			}
			return c.offerRemoveOverrides(g)
		},
	}
}

// identityKeys are the settings a profile controls; a local value overrides it.
var identityKeys = []string{"user.name", "user.email", "user.signingKey", "gpg.format", "core.sshCommand", "credential.username"}

// localOverrides lists "key = value" for identity keys set in .git/config.
func localOverrides(g git.Client) ([]string, error) {
	var out []string
	for _, k := range identityKeys {
		v, ok, err := g.ConfigGet(k, "--local")
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, k+" = "+v)
		}
	}
	return out, nil
}

// offerRemoveOverrides offers to delete identity settings in .git/config that
// would beat the profile (e.g. left by gids v1 or a manual git config).
func (c *cli) offerRemoveOverrides(g git.Client) error {
	ov, err := localOverrides(g)
	if err != nil || len(ov) == 0 {
		return err
	}
	c.ui.Warn("This repo's .git/config overrides the profile:\n    %s", strings.Join(ov, "\n    "))
	ok, err := c.ui.Confirm("Remove these local settings?", true)
	if err != nil || !ok {
		return err
	}
	for _, k := range identityKeys {
		if err := g.ConfigUnsetLocal(k); err != nil {
			return err
		}
	}
	c.ui.Success("Removed local overrides.")
	return nil
}
