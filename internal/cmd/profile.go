package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/config"
)

// configPath is used by profile sub-commands; overridable in tests.
var configPath string

func printProfileTable(w io.Writer, profiles []config.Profile) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tGIT NAME\tGIT EMAIL\tUSERNAME\tSSH KEY")
	for _, p := range profiles {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			p.Name, p.GitName, p.GitEmail, p.Username, p.SSHKey)
	}
	tw.Flush()
}

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage Git identity profiles",
	}
	cmd.PersistentFlags().StringVar(&configPath, "config", "", "path to config file (default: $UserConfigDir/gids/config.yaml)")
	cmd.AddCommand(newProfileAddCmd())
	cmd.AddCommand(newProfileListCmd())
	cmd.AddCommand(newProfileEditCmd())
	cmd.AddCommand(newProfileDeleteCmd())
	return cmd
}

// prompt prints a message and reads a trimmed line from stdin.
func prompt(reader *bufio.Reader, message string) (string, error) {
	fmt.Fprint(os.Stdout, message)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func newProfileAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Add a new identity profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			reader := bufio.NewReader(os.Stdin)

			name, err := prompt(reader, "Profile name (e.g. Work, Personal): ")
			if err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("profile name is required")
			}
			if p, _ := cfg.FindProfile(name); p != nil {
				return fmt.Errorf("profile %q already exists", name)
			}

			gitName, err := prompt(reader, "Git name: ")
			if err != nil {
				return err
			}
			if gitName == "" {
				return fmt.Errorf("git name is required")
			}

			gitEmail, err := prompt(reader, "Git email: ")
			if err != nil {
				return err
			}
			if gitEmail == "" {
				return fmt.Errorf("git email is required")
			}

			username, err := prompt(reader, "Username (optional, for HTTPS — sets credential.username, Enter to skip): ")
			if err != nil {
				return err
			}

			sshKey, err := prompt(reader, "SSH key path (optional, e.g. ~/.ssh/id_work, Enter to skip): ")
			if err != nil {
				return err
			}

			signingKey, err := prompt(reader, "Signing key (optional, GPG fingerprint or SSH key path, Enter to skip): ")
			if err != nil {
				return err
			}

			if username == "" && sshKey == "" && signingKey == "" {
				fmt.Fprintln(os.Stdout, "No auth method set — push/pull will use your system default.")
			}

			cfg.Profiles = append(cfg.Profiles, config.Profile{
				Name:       name,
				GitName:    gitName,
				GitEmail:   gitEmail,
				Username:   username,
				SSHKey:     sshKey,
				SigningKey:  signingKey,
			})

			if err := config.Save(cfg, configPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Profile %q added.\n", name)
			return nil
		},
	}
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all identity profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(os.Stdout, "No profiles found. Run 'gids profile add' to create one.")
				return nil
			}

			printProfileTable(os.Stdout, cfg.Profiles)
			return nil
		},
	}
}

func newProfileEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit an existing identity profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			p, _ := cfg.FindProfile(name)
			if p == nil {
				return fmt.Errorf("profile %q not found", name)
			}

			reader := bufio.NewReader(os.Stdin)

			gitName, err := prompt(reader, fmt.Sprintf("Git name [%s]: ", p.GitName))
			if err != nil {
				return err
			}
			if gitName == "" {
				gitName = p.GitName
			}

			gitEmail, err := prompt(reader, fmt.Sprintf("Git email [%s]: ", p.GitEmail))
			if err != nil {
				return err
			}
			if gitEmail == "" {
				gitEmail = p.GitEmail
			}

			username, err := prompt(reader, fmt.Sprintf("Username [%s] (Enter to keep, \"none\" to clear): ", p.Username))
			if err != nil {
				return err
			}
			switch username {
			case "none":
				username = ""
			case "":
				username = p.Username
			}

			sshKey, err := prompt(reader, fmt.Sprintf("SSH key path [%s] (Enter to keep, \"none\" to clear): ", p.SSHKey))
			if err != nil {
				return err
			}
			switch sshKey {
			case "none":
				sshKey = ""
			case "":
				sshKey = p.SSHKey
			}

			signingKey, err := prompt(reader, fmt.Sprintf("Signing key [%s] (Enter to keep, \"none\" to clear): ", p.SigningKey))
			if err != nil {
				return err
			}
			switch signingKey {
			case "none":
				signingKey = ""
			case "":
				signingKey = p.SigningKey
			}

			if username == "" && sshKey == "" && signingKey == "" {
				fmt.Fprintln(os.Stdout, "No auth method set — push/pull will use your system default.")
			}

			p.GitName = gitName
			p.GitEmail = gitEmail
			p.Username = username
			p.SSHKey = sshKey
			p.SigningKey = signingKey

			if err := config.Save(cfg, configPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Profile %q updated.\n", name)
			return nil
		},
	}
}

func newProfileDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete an identity profile",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if p, _ := cfg.FindProfile(name); p == nil {
				return fmt.Errorf("profile %q not found", name)
			}

			if !force {
				reader := bufio.NewReader(os.Stdin)
				answer, err := prompt(reader, fmt.Sprintf("Delete profile %q? [y/N] ", name))
				if err != nil {
					return err
				}
				if strings.ToLower(answer) != "y" {
					fmt.Fprintln(os.Stdout, "Aborted.")
					return nil
				}
			}

			cfg.DeleteProfile(name)

			if err := config.Save(cfg, configPath); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Profile %q deleted.\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	return cmd
}
