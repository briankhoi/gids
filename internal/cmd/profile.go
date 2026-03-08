package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
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

func warnNoAuth(w io.Writer, username, sshKey, signingKey string) {
	if username == "" && sshKey == "" && signingKey == "" {
		fmt.Fprintln(w, "No auth method set - push/pull will use your system default.")
	}
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

			out := cmd.OutOrStdout()
			r := bufio.NewReader(cmd.InOrStdin())

			name, err := prompt(r, out, "Profile name (e.g. Work, Personal): ")
			if err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("profile name is required")
			}
			if p, _ := cfg.FindProfile(name); p != nil {
				return fmt.Errorf("profile %q already exists", name)
			}

			gitName, err := prompt(r, out, "Git name: ")
			if err != nil {
				return err
			}
			if gitName == "" {
				return fmt.Errorf("git name is required")
			}

			gitEmail, err := prompt(r, out, "Git email: ")
			if err != nil {
				return err
			}
			if gitEmail == "" {
				return fmt.Errorf("git email is required")
			}

			username, err := prompt(r, out, "Username (optional, for HTTPS — sets credential.username, Enter to skip): ")
			if err != nil {
				return err
			}

			sshKey, err := prompt(r, out, "SSH key path (optional, e.g. ~/.ssh/id_work, Enter to skip): ")
			if err != nil {
				return err
			}

			signingKey, err := prompt(r, out, "Signing key (optional, GPG fingerprint or SSH key path, Enter to skip): ")
			if err != nil {
				return err
			}

			warnNoAuth(out, username, sshKey, signingKey)

			cfg.Profiles = append(cfg.Profiles, config.Profile{
				Name:       name,
				GitName:    gitName,
				GitEmail:   gitEmail,
				Username:   username,
				SSHKey:     sshKey,
				SigningKey: signingKey,
			})

			if err := config.Save(cfg, *cfgPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(out, "Profile %q added.\n", name)
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

			// Resolve which SSH config file to use.
			resolvedPath, err := resolveSSHConfigPath(r, out, filePath)
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "\nParsing %s...\n\n", tildify(resolvedPath))
			hosts, err := sshconfig.ParseFile(resolvedPath)
			if err != nil {
				return err
			}

			// Apply --host filter if set.
			if hostFilter != "" {
				var filtered []sshconfig.Host
				lower := strings.ToLower(hostFilter)
				for _, h := range hosts {
					if strings.Contains(strings.ToLower(h.Pattern), lower) {
						filtered = append(filtered, h)
					}
				}
				hosts = filtered
			}

			if len(hosts) == 0 {
				fmt.Fprintf(out, "No SSH host entries found in %s.\n", resolvedPath)
				return nil
			}

			// Display found hosts.
			fmt.Fprintf(out, "Found %d host entr%s:\n", len(hosts), pluralSuffix(len(hosts), "y", "ies"))
			for i, h := range hosts {
				fmt.Fprintf(out, "  [%d] %-20s IdentityFile: %-25s User: %s\n",
					i+1, h.Pattern, h.IdentityFile, h.User)
			}
			fmt.Fprintln(out)

			// Determine which hosts to import.
			toImport, err := selectHostsToImport(r, out, hosts)
			if err != nil {
				return err
			}
			if len(toImport) == 0 {
				fmt.Fprintln(out, "Nothing to import.")
				return nil
			}

			// Load config once.
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			var created []config.Profile
			var lastGitName string

			for _, h := range toImport {
				fmt.Fprintf(out, "\n--- Importing %q ---\n", h.Pattern)

				// Skip if profile already exists.
				if p, _ := cfg.FindProfile(h.Pattern); p != nil {
					fmt.Fprintf(out, "Profile %q already exists, skipping.\n", h.Pattern)
					continue
				}

				// Warn if no IdentityFile.
				if h.IdentityFile == "" {
					fmt.Fprintf(out, "No IdentityFile set for %q — SSH key will be empty.\n", h.Pattern)
				}

				// Prompt for required Git name.
				namePrompt := "Git name (required): "
				if lastGitName != "" {
					namePrompt = fmt.Sprintf("Git name [%s] (Enter to reuse, or type new): ", lastGitName)
				}
				gitName, err := promptRequired(r, out, namePrompt, lastGitName)
				if err != nil {
					return err
				}
				lastGitName = gitName

				// Prompt for required Git email.
				gitEmail, err := promptRequired(r, out, "Git email (required): ", "")
				if err != nil {
					return err
				}

				p := config.Profile{
					Name:     h.Pattern,
					GitName:  gitName,
					GitEmail: gitEmail,
					Username: h.User,
					SSHKey:   h.IdentityFile,
				}
				cfg.Profiles = append(cfg.Profiles, p)
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

// resolveSSHConfigPath returns the SSH config path to use. If filePath is set,
// it's used directly. Otherwise, the user is prompted to pick from known paths.
func resolveSSHConfigPath(r *bufio.Reader, w io.Writer, filePath string) (string, error) {
	if filePath != "" {
		return filePath, nil
	}

	candidates, err := sshconfig.DefaultConfigPaths()
	if err != nil {
		return "", fmt.Errorf("resolving SSH config paths: %w", err)
	}
	fmt.Fprintln(w, "Known SSH config locations:")
	for i, p := range candidates {
		status := "not found"
		if _, err := os.Stat(p); err == nil {
			status = "found"
		}
		fmt.Fprintf(w, "  [%d] %-35s (%s)\n", i+1, tildify(p), status)
	}
	fmt.Fprintln(w, "  [0] Enter a custom path")
	fmt.Fprintln(w)

	// Build prompt showing the full valid range, e.g. "Enter 0–2 [default: 1]: "
	max := len(candidates)
	for {
		val, err := prompt(r, w, fmt.Sprintf("Enter 0-%d [default: 1]: ", max))
		if err != nil {
			return "", err
		}
		if val == "" {
			val = "1"
		}

		if val == "0" {
			customPath, err := promptRequired(r, w, "SSH config path: ", "")
			if err != nil {
				return "", err
			}
			return customPath, nil
		}

		// Validate and map to a candidate.
		if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= max {
			return candidates[n-1], nil
		}
		fmt.Fprintf(w, "Please enter a number between 0 and %d.\n", max)
	}
}

// selectHostsToImport asks the user which hosts to import, returning the selection.
func selectHostsToImport(r *bufio.Reader, w io.Writer, hosts []sshconfig.Host) ([]sshconfig.Host, error) {
	all, err := confirmPrompt(r, w, "Import all?", true)
	if err != nil {
		return nil, err
	}
	if all {
		return hosts, nil
	}

	var selected []sshconfig.Host
	for _, h := range hosts {
		ok, err := confirmPrompt(r, w, fmt.Sprintf("Import %q?", h.Pattern), false)
		if err != nil {
			return nil, err
		}
		if ok {
			selected = append(selected, h)
		}
	}
	return selected, nil
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

// editedProfile returns a new Profile with the provided field values applied to
// base. The Name field is preserved; no fields of base are mutated.
func editedProfile(base config.Profile, gitName, gitEmail, username, sshKey, signingKey string) config.Profile {
	return config.Profile{
		Name:       base.Name,
		GitName:    gitName,
		GitEmail:   gitEmail,
		Username:   username,
		SSHKey:     sshKey,
		SigningKey:  signingKey,
	}
}

// pluralSuffix returns singular if n==1, plural otherwise.
func pluralSuffix(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
