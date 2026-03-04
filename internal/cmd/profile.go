package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
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

// prompt writes message to w and reads a trimmed line from r.
func prompt(r *bufio.Reader, w io.Writer, message string) (string, error) {
	fmt.Fprint(w, message)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptOptional prompts for an optional field with keep/clear semantics:
// empty input keeps current; "none" clears it.
func promptOptional(r *bufio.Reader, w io.Writer, label, current string) (string, error) {
	val, err := prompt(r, w, fmt.Sprintf("%s [%s] (Enter to keep, \"none\" to clear): ", label, current))
	if err != nil {
		return "", err
	}
	switch val {
	case "none":
		return "", nil
	case "":
		return current, nil
	default:
		return val, nil
	}
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
				Name:      name,
				GitName:   gitName,
				GitEmail:  gitEmail,
				Username:  username,
				SSHKey:    sshKey,
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

			p, _ := cfg.FindProfile(name)
			if p == nil {
				return fmt.Errorf("profile %q not found", name)
			}

			out := cmd.OutOrStdout()
			r := bufio.NewReader(cmd.InOrStdin())

			gitName, err := prompt(r, out, fmt.Sprintf("Git name [%s]: ", p.GitName))
			if err != nil {
				return err
			}
			if gitName == "" {
				gitName = p.GitName
			}

			gitEmail, err := prompt(r, out, fmt.Sprintf("Git email [%s]: ", p.GitEmail))
			if err != nil {
				return err
			}
			if gitEmail == "" {
				gitEmail = p.GitEmail
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

			p.GitName = gitName
			p.GitEmail = gitEmail
			p.Username = username
			p.SSHKey = sshKey
			p.SigningKey = signingKey

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
				answer, err := prompt(r, out, fmt.Sprintf("Delete profile %q? [y/N] ", name))
				if err != nil {
					return err
				}
				if strings.ToLower(answer) != "y" {
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
