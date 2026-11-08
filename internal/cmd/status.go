package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
)

func newStatusCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which identity applies here, and why",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, err := c.env.Installed()
			if err != nil {
				return err
			}
			cfg, err := c.load()
			if err != nil {
				return err
			}
			g, r, err := c.repo()
			if errors.Is(err, git.ErrNotRepo) {
				c.ui.Printf("Not in a git repository. %d profile(s), %d rule(s).\n", len(cfg.Profiles), len(cfg.Rules))
				if !installed {
					c.ui.Warn("gids isn't installed. Run: gids install")
				}
				return nil
			}
			if err != nil {
				return err
			}
			src, hasEmail, err := g.ConfigSource("user.email")
			if err != nil {
				return err
			}
			origin, email := src.Origin, src.Value
			name, _, err := g.ConfigGet("user.name")
			if err != nil {
				return err
			}
			profile, mapped, err := g.ConfigGet("gids.profile")
			if err != nil {
				return err
			}
			coauthors, err := g.ConfigGetAll("gids.coauthor", "--local")
			if err != nil {
				return err
			}

			tw := tabwriter.NewWriter(c.ui.Out(), 0, 0, 2, ' ', 0)
			if hasEmail {
				fmt.Fprintf(tw, "Identity:\t%s <%s>\t(from %s)\n", name, email, c.showOrigin(origin))
			} else {
				fmt.Fprintf(tw, "Identity:\t(none set)\t\n")
			}
			p := cfg.Profile(profile)
			switch {
			case mapped && p != nil:
				fmt.Fprintf(tw, "Profile:\t%s\t(rule %s)\n", p.Name, ruleFor(cfg, p.Name, r.RuleDir(), c.env.Paths.Home))
			default:
				fmt.Fprintf(tw, "Profile:\t(none: no rule covers this repo)\t\n")
			}
			if len(coauthors) > 0 {
				fmt.Fprintf(tw, "Co-authors:\t%s\t\n", strings.Join(coauthors, ", "))
			}
			if err := tw.Flush(); err != nil {
				return err
			}

			if !installed {
				c.ui.Warn("gids isn't installed, so rules don't apply. Run: gids install")
			}
			if p != nil && hasEmail && !strings.EqualFold(email, p.GitEmail) {
				c.ui.Warn("The identity is overridden (%s), not %s from %s. Fix with: gids use %s", c.showOrigin(origin), p.GitEmail, p.Name, p.Name)
			}
			if hp, ok, _ := g.ConfigGet("core.hooksPath", "--local"); ok {
				c.ui.Warn("This repo sets its own core.hooksPath (%s), so gids' commit guard doesn't run here.", hp)
			}
			if installed {
				c.warnHooksOverridden()
			}
			return nil
		},
	}
}

// showOrigin turns --show-origin output into something short.
func (c *cli) showOrigin(origin string) string {
	if f, ok := strings.CutPrefix(origin, "file:"); ok {
		if !filepath.IsAbs(f) { // repo-relative, i.e. .git/config
			return f
		}
		return c.env.Display(f)
	}
	return origin
}

// ruleFor finds the deepest rule of profile that contains dir (literal
// prefix match on normalized keys; glob rules are shown as "matching").
func ruleFor(cfg *config.Config, profile, dir, home string) string {
	key, err := config.NormalizeRule(dir, dir, home)
	if err != nil {
		return "matching pattern"
	}
	best := ""
	for _, k := range cfg.RulesFor(profile) {
		if strings.HasPrefix(key, k) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return "matching pattern"
	}
	return best
}
