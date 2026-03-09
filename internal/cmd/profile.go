package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/sshconfig"
)

func newProfileCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage Git identity profiles",
	}
	cmd.PersistentFlags().StringVar(&cfgPath, "config", "", "path to config file (default: $UserConfigDir/gids/config.yaml)")
	cmd.AddCommand(newProfileAddCmd(&cfgPath))
	cmd.AddCommand(newProfileListCmd(&cfgPath))
	cmd.AddCommand(newProfileEditCmd(&cfgPath))
	cmd.AddCommand(newProfileDeleteCmd(&cfgPath))
	cmd.AddCommand(newProfileImportCmd(&cfgPath))
	return cmd
}

func printProfileTable(w io.Writer, profiles []config.Profile) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tGIT NAME\tGIT EMAIL\tUSERNAME\tSSH KEY")
	for _, p := range profiles {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			p.Name, p.GitName, p.GitEmail, p.Username, p.SSHKey)
	}
	tw.Flush()
}

func newProfileAddCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Add a new identity profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			p, err := buildProfileFromPrompts(bufio.NewReader(cmd.InOrStdin()), cmd.OutOrStdout(), cfg)
			if err != nil {
				return err
			}

			cfg.Profiles = append(cfg.Profiles, p)

			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Profile %q added.\n", p.Name)
			return nil
		},
	}
}

func newProfileListCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all identity profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(out, "No profiles found. Run 'gids profile add' to create one.")
				return nil
			}

			printProfileTable(out, cfg.Profiles)
			return nil
		},
	}
}

func newProfileEditCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit an existing identity profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			p, idx := cfg.FindProfile(name)
			if p == nil {
				return fmt.Errorf("profile %q not found", name)
			}

			out := cmd.OutOrStdout()
			r := bufio.NewReader(cmd.InOrStdin())

			gitName, err := promptRequired(r, out, fmt.Sprintf("Git name [%s]: ", p.GitName), p.GitName)
			if err != nil {
				return err
			}

			gitEmail, err := promptRequired(r, out, fmt.Sprintf("Git email [%s]: ", p.GitEmail), p.GitEmail)
			if err != nil {
				return err
			}

			username, err := promptOptional(r, out, "Username", p.Username)
			if err != nil {
				return err
			}

			sshKey, err := promptOptional(r, out, "SSH key path", p.SSHKey)
			if err != nil {
				return err
			}

			signingKey, err := promptOptional(r, out, "Signing key", p.SigningKey)
			if err != nil {
				return err
			}

			warnNoAuth(out, username, sshKey, signingKey)

			cfg.Profiles[idx] = editedProfile(*p, gitName, gitEmail, username, sshKey, signingKey)

			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(out, "Profile %q updated.\n", name)
			return nil
		},
	}
}

func newProfileDeleteCmd(cfgPath *string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete an identity profile",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if p, _ := cfg.FindProfile(name); p == nil {
				return fmt.Errorf("profile %q not found", name)
			}

			out := cmd.OutOrStdout()
			if !force {
				r := bufio.NewReader(cmd.InOrStdin())
				ok, err := confirmPrompt(r, out, fmt.Sprintf("Delete profile %q?", name), false)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "Aborted.")
					return nil
				}
			}

			cfg.DeleteProfile(name)

			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(out, "Profile %q deleted.\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	return cmd
}

func newProfileImportCmd(cfgPath *string) *cobra.Command {
	var filePath string
	var hostFilter string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import profiles from an SSH config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			r := bufio.NewReader(cmd.InOrStdin())

			resolvedPath, err := resolveSSHConfigPath(r, out, filePath)
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "\nParsing %s...\n\n", tildify(resolvedPath))
			hosts, err := sshconfig.ParseFile(resolvedPath)
			if err != nil {
				return err
			}

			hosts = filterHosts(hosts, hostFilter)

			if len(hosts) == 0 {
				fmt.Fprintf(out, "No SSH host entries found in %s.\n", resolvedPath)
				return nil
			}

			fmt.Fprintf(out, "Found %d host entr%s:\n", len(hosts), pluralSuffix(len(hosts), "y", "ies"))
			for i, h := range hosts {
				fmt.Fprintf(out, "  [%d] %-20s IdentityFile: %-25s User: %s\n",
					i+1, h.Pattern, h.IdentityFile, h.User)
			}
			fmt.Fprintln(out)

			toImport, err := selectHostsToImport(r, out, hosts)
			if err != nil {
				return err
			}
			if len(toImport) == 0 {
				fmt.Fprintln(out, "Nothing to import.")
				return nil
			}

			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			var created []config.Profile
			var lastGitName string

			for _, h := range toImport {
				fmt.Fprintf(out, "\n--- Importing %q ---\n", h.Pattern)
				p, ok, err := importHost(r, out, cfg, h, lastGitName)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				lastGitName = p.GitName
				created = append(created, p)
				fmt.Fprintf(out, "Profile %q created.\n", h.Pattern)
			}

			if len(created) == 0 {
				return nil
			}

			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(out, "\nCreated %d profile(s):\n", len(created))
			printProfileTable(out, created)

			fmt.Fprintln(out, "\nTo modify a profile, run:")
			for _, p := range created {
				fmt.Fprintf(out, "  gids profile edit %q\n", p.Name)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "SSH config file to import from (skips discovery)")
	cmd.Flags().StringVar(&hostFilter, "host", "", "import only hosts whose pattern contains this string (case-insensitive)")
	return cmd
}

// homeDir returns the current user's home directory.
func homeDir() (string, error) {
	return os.UserHomeDir()
}

// tildify replaces the home directory prefix with ~.
// Falls back to returning path unchanged if the home directory cannot be
// determined — this is acceptable because tildify is display-only.
func tildify(path string) string {
	home, err := homeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	if path == home {
		return "~"
	}
	return path
}

// pluralSuffix returns singular if n==1, plural otherwise.
func pluralSuffix(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
