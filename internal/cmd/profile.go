package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/sshconfig"
	"gids/internal/ui"
)

func newProfileCmd(c *cli) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage identity profiles"}
	cmd.AddCommand(newProfileAddCmd(c), newProfileListCmd(c), newProfileEditCmd(c), newProfileDeleteCmd(c), newProfileImportCmd(c))
	return cmd
}

// profileFlags lets every field be given on the command line (scripts,
// tests); the form only asks for what's missing or invalid.
type profileFlags struct {
	name, gitName, email, username, sshKey, signingKey string
}

func (f *profileFlags) register(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.name, "name", "", "profile name (e.g. Work)")
	fl.StringVar(&f.gitName, "git-name", "", "git author name")
	fl.StringVar(&f.email, "email", "", "git author email")
	fl.StringVar(&f.username, "username", "", "HTTPS username (credential.username)")
	fl.StringVar(&f.sshKey, "ssh-key", "", "SSH private key path (core.sshCommand)")
	fl.StringVar(&f.signingKey, "signing-key", "", "GPG key id or SSH key path for signing")
}

// overlay applies flags the user set onto p.
func (f *profileFlags) overlay(cmd *cobra.Command, p config.Profile) config.Profile {
	set := func(flag string, dst *string, v string) {
		if cmd.Flags().Changed(flag) {
			*dst = v
		}
	}
	set("name", &p.Name, f.name)
	set("git-name", &p.GitName, f.gitName)
	set("email", &p.GitEmail, f.email)
	set("username", &p.Username, f.username)
	set("ssh-key", &p.SSHKey, f.sshKey)
	set("signing-key", &p.SigningKey, f.signingKey)
	return p
}

// profileForm asks for every field (pre-filled with p) and returns the result.
// nameTaken rejects names already used by another profile.
func (c *cli) profileForm(title string, p config.Profile, nameTaken func(string) bool) (config.Profile, error) {
	vals, err := c.ui.Form(title, []ui.Field{
		{Label: "Name", Value: p.Name, Validate: func(s string) error {
			if err := config.ValidateProfileName(s); err != nil {
				return err
			}
			if nameTaken(s) {
				return fmt.Errorf("a profile named %q already exists", s)
			}
			return nil
		}},
		{Label: "Git name", Value: p.GitName, Validate: config.ValidateGitName},
		{Label: "Git email", Value: p.GitEmail, Validate: config.ValidateEmail},
		{Label: "Username", Value: p.Username, Optional: true, Validate: config.ValidateUsername},
		{Label: "SSH key", Value: p.SSHKey, Optional: true, Validate: config.ValidateKeyPath, Hint: c.fileHint},
		{Label: "Signing key", Value: p.SigningKey, Optional: true, Validate: config.ValidateSigningKey},
	})
	if err != nil {
		return config.Profile{}, err
	}
	return config.Profile{Name: vals[0], GitName: vals[1], GitEmail: vals[2], Username: vals[3], SSHKey: vals[4], SigningKey: vals[5]}, nil
}

// fileHint says whether an SSH key path exists, as a live form hint.
func (c *cli) fileHint(path string) string {
	if path == "" || config.ValidateKeyPath(path) != nil {
		return ""
	}
	if _, err := os.Stat(c.env.Expand(path)); err != nil {
		return "⚠ not found"
	}
	return "✓ file exists"
}

func newProfileAddCmd(c *cli) *cobra.Command {
	var f profileFlags
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a profile (asks for anything not given as a flag)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := c.load()
			if err != nil {
				return err
			}
			p := f.overlay(cmd, config.Profile{})
			if p.Validate() != nil || cfg.ProfileFold(p.Name) != nil {
				taken := func(s string) bool { return cfg.ProfileFold(s) != nil }
				if p, err = c.profileForm("New profile", p, taken); err != nil {
					return err
				}
			}
			if _, err := c.env.Mutate(func(cfg *config.Config) error { return cfg.AddProfile(p) }); err != nil {
				return err
			}
			c.ui.Success("Profile %q saved (%s).", p.Name, p.Ident())
			if p.SSHKey == "" && p.Username == "" {
				c.ui.Hint("No SSH key or username set: push/pull use your system defaults.")
			}
			if len(cfg.Rules) == 0 {
				c.ui.Hint("Map it to a directory: gids rule add ~/work %s   (or 'gids use %s' inside a repo)", p.Name, p.Name)
			}
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

func newProfileListCmd(c *cli) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List profiles",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := c.load()
			if err != nil {
				return err
			}
			if len(cfg.Profiles) == 0 {
				c.ui.Printf("No profiles yet. Add one: gids profile add\n")
				return nil
			}
			printProfiles(c, cfg, cfg.Profiles)
			return nil
		},
	}
}

func printProfiles(c *cli, cfg *config.Config, ps []config.Profile) {
	tw := tabwriter.NewWriter(c.ui.Out(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tGIT NAME\tEMAIL\tSSH KEY\tRULES")
	for _, p := range ps {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n", p.Name, p.GitName, p.GitEmail, dash(p.SSHKey), len(cfg.RulesFor(p.Name)))
	}
	tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newProfileEditCmd(c *cli) *cobra.Command {
	var f profileFlags
	cmd := &cobra.Command{
		Use:               "edit <name>",
		Short:             "Edit a profile (flags change fields directly; otherwise a form)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: c.completeProfiles,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := c.load()
			if err != nil {
				return err
			}
			old := cfg.ProfileFold(args[0])
			if old == nil {
				return fmt.Errorf("profile %q not found", args[0])
			}
			oldName := old.Name
			p := f.overlay(cmd, *old)
			if cmd.Flags().NFlag() == 0 || p.Validate() != nil {
				taken := func(s string) bool { q := cfg.ProfileFold(s); return q != nil && q.Name != oldName }
				if p, err = c.profileForm("Edit profile "+oldName, p, taken); err != nil {
					return err
				}
			}
			if _, err := c.env.Mutate(func(cfg *config.Config) error { return cfg.ReplaceProfile(oldName, p) }); err != nil {
				return err
			}
			if p.Name != oldName {
				c.ui.Success("Profile %q renamed to %q and updated; its rules follow it.", oldName, p.Name)
			} else {
				c.ui.Success("Profile %q updated.", p.Name)
			}
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

func newProfileDeleteCmd(c *cli) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:               "delete <name>",
		Aliases:           []string{"rm"},
		Short:             "Delete a profile",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: c.completeProfiles,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := c.load()
			if err != nil {
				return err
			}
			p := cfg.ProfileFold(args[0])
			if p == nil {
				return fmt.Errorf("profile %q not found", args[0])
			}
			name, rules := p.Name, cfg.RulesFor(p.Name)
			if len(rules) > 0 && !force {
				return fmt.Errorf("profile %q is used by %d rule(s): %s\nRemove them first, or use --force to delete the profile and its rules", name, len(rules), strings.Join(rules, ", "))
			}
			if !force {
				ok, err := c.ui.Confirm(fmt.Sprintf("Delete profile %q?", name), false)
				if err != nil {
					return err
				}
				if !ok {
					c.ui.Printf("Kept %q.\n", name)
					return nil
				}
			}
			if _, err := c.env.Mutate(func(cfg *config.Config) error {
				if cfg.Profile(name) == nil {
					return fmt.Errorf("profile %q not found", name)
				}
				cfg.DeleteProfile(name)
				return nil
			}); err != nil {
				return err
			}
			c.ui.Success("Deleted profile %q%s.", name, plural(len(rules), " and its rule", " and its %d rules"))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "don't ask; also delete rules that use the profile")
	return cmd
}

// plural formats n with one/many variants ("" when n is 0).
func plural(n int, one, many string) string {
	switch n {
	case 0:
		return ""
	case 1:
		return one
	}
	if strings.Contains(many, "%d") {
		return fmt.Sprintf(many, n)
	}
	return many
}

func (c *cli) completeProfiles(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load(c.envPathsConfig())
	if err != nil || len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, p := range cfg.Profiles {
		names = append(names, p.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// envPathsConfig works during completion, before PersistentPreRun ran.
func (c *cli) envPathsConfig() string {
	if c.env != nil {
		return c.env.Paths.Config
	}
	p, err := config.ResolvePaths()
	if err != nil {
		return ""
	}
	return p.Config
}

func newProfileImportCmd(c *cli) *cobra.Command {
	var file, host string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Create profiles from Host blocks in an SSH config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				var err error
				if file, err = c.pickSSHConfig(); err != nil {
					return err
				}
			}
			hosts, err := sshconfig.ParseFile(c.env.Expand(file))
			if err != nil {
				return err
			}
			hosts = sshconfig.Filter(hosts, host)
			if len(hosts) == 0 {
				c.ui.Printf("No SSH host entries found in %s.\n", c.env.Display(file))
				return nil
			}
			items := make([]ui.Item, len(hosts))
			checked := make([]bool, len(hosts))
			for i, h := range hosts {
				items[i] = ui.Item{Label: h.Pattern, Detail: strings.TrimSpace(dash(h.IdentityFile) + "  user " + dash(h.User))}
				checked[i] = true
			}
			sel, err := c.ui.MultiSelect("Hosts to import", items, checked)
			if err != nil {
				return err
			}
			var created []config.Profile
			lastName := ""
			for i, h := range hosts {
				if !sel[i] {
					continue
				}
				p, ok, err := c.importHost(h, lastName)
				if errors.Is(err, ui.ErrSkipped) {
					c.ui.Hint("Skipped %q.", h.Pattern)
					continue
				}
				if err != nil {
					return err
				}
				if ok {
					created = append(created, p)
					lastName = p.GitName
				}
			}
			if len(created) == 0 {
				c.ui.Printf("Nothing imported.\n")
				return nil
			}
			cfg, err := c.load()
			if err != nil {
				return err
			}
			c.ui.Success("Imported %d profile(s):", len(created))
			printProfiles(c, cfg, created)
			c.ui.Hint("Adjust any of them with: gids profile edit <name>")
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "SSH config file (default: choose from the usual locations)")
	cmd.Flags().StringVar(&host, "host", "", "only hosts whose pattern contains this text (case-insensitive)")
	return cmd
}

// importHost asks for the git identity of one SSH host and saves the profile.
func (c *cli) importHost(h sshconfig.Host, lastName string) (config.Profile, bool, error) {
	cfg, err := c.load()
	if err != nil {
		return config.Profile{}, false, err
	}
	name := h.Pattern
	if config.ValidateProfileName(name) != nil || cfg.ProfileFold(name) != nil {
		if cfg.ProfileFold(name) != nil {
			c.ui.Warn("Profile %q already exists, skipping.", name)
			return config.Profile{}, false, nil
		}
		c.ui.Hint("%q can't be a profile name.", name)
		name, err = c.ui.Input("Profile name for "+h.Pattern, "", func(s string) error {
			if err := config.ValidateProfileName(s); err != nil {
				return err
			}
			if cfg.ProfileFold(s) != nil {
				return fmt.Errorf("a profile named %q already exists", s)
			}
			return nil
		})
		if err != nil {
			return config.Profile{}, false, err
		}
	}
	if h.IdentityFile == "" {
		c.ui.Warn("No IdentityFile set for %q — SSH key will be empty.", h.Pattern)
	}
	p := config.Profile{Name: name, Username: h.User, SSHKey: h.IdentityFile}
	if config.ValidateKeyPath(p.SSHKey) != nil { // e.g. a relative IdentityFile
		c.ui.Warn("IdentityFile %q isn't an absolute or ~/ path; leaving SSH key empty.", p.SSHKey)
		p.SSHKey = ""
	}
	if config.ValidateUsername(p.Username) != nil {
		p.Username = ""
	}
	c.ui.Printf("%s\n", c.ui.S.Title.Render("Importing "+h.Pattern))
	if p.GitName, err = c.ui.Input("Git name", lastName, config.ValidateGitName); err != nil {
		return config.Profile{}, false, err
	}
	if p.GitEmail, err = c.ui.Input("Git email", "", config.ValidateEmail); err != nil {
		return config.Profile{}, false, err
	}
	if _, err := c.env.Mutate(func(cfg *config.Config) error { return cfg.AddProfile(p) }); err != nil {
		return config.Profile{}, false, err
	}
	return p, true, nil
}

// pickSSHConfig lets the user choose a known SSH config path or type one.
func (c *cli) pickSSHConfig() (string, error) {
	paths, err := sshconfig.DefaultConfigPaths()
	if err != nil {
		return "", err
	}
	var items []ui.Item
	for _, p := range paths {
		status := "not found"
		if _, err := os.Stat(p); err == nil {
			status = "found"
		}
		items = append(items, ui.Item{Label: c.env.Display(p), Detail: status})
	}
	items = append(items, ui.Item{Label: "Other…", Detail: "enter a path"})
	i, err := c.ui.Select("SSH config to import from", items)
	if err != nil {
		return "", err
	}
	if i < len(paths) {
		return paths[i], nil
	}
	return c.ui.Input("SSH config path", "", func(s string) error {
		if s == "" {
			return errors.New("a path is required")
		}
		return nil
	})
}
