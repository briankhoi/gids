package cmd

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"gids/internal/git"
)

func newInstallCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Hook gids into your global git config (once per machine)",
		Long: `Adds one marked block to your global git config (~/.gitconfig) that includes
gids' generated config and points core.hooksPath at gids' hooks, which chain to
your repos' own hooks. Safe to run again. Undo with 'gids uninstall'.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			changed, err := c.env.Install()
			if err != nil {
				return err
			}
			u, p := c.ui, c.env.Paths
			if changed {
				u.Success("Installed: %s now includes gids.", c.env.Display(p.GlobalGitconfig))
			} else {
				u.Success("Already installed.")
			}
			c.warnHooksOverridden()
			if transientExe(c.env.Exe) {
				u.Warn("gids is running from a temporary 'go run' build (%s); the hooks will stop finding it once Go deletes it. Install a real binary (go install .) and run 'gids install' again.", c.env.Exe)
			}
			if cfg, err := c.load(); err == nil && len(cfg.Profiles) == 0 {
				u.Hint("Next: gids profile add")
			}
			return nil
		},
	}
}

func newUninstallCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove gids from your global git config (keeps your profiles)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			changed, err := c.env.Uninstall()
			if err != nil {
				return err
			}
			if changed {
				c.ui.Success("Uninstalled. Your profiles stay in %s.", c.env.Display(c.env.Paths.Config))
			} else {
				c.ui.Success("Not installed; nothing to do.")
			}
			return nil
		},
	}
}

// warnHooksOverridden warns when a later core.hooksPath setting in the global
// config beats gids' (then the guard never runs).
func (c *cli) warnHooksOverridden() {
	v, ok, err := git.Client{Dir: c.env.Paths.Home}.ConfigGet("core.hooksPath", "--global")
	if err != nil || !ok {
		return
	}
	if filepath.Clean(c.env.Expand(v)) != filepath.Clean(c.env.Paths.HooksDir) {
		c.ui.Warn("core.hooksPath = %s is set after gids' block in your global git config, so gids' hooks won't run. Remove it, or run 'gids install' again to move gids' block last.", v)
	}
}

// transientExe reports whether exe is a 'go run' build, which Go deletes
// when the run ends: hooks baked with that path would silently stop working
// (they fail open, so the guard would just vanish).
func transientExe(exe string) bool {
	p := filepath.ToSlash(exe)
	return strings.Contains(p, "/go-build") && strings.Contains(p, "/exe/")
}
