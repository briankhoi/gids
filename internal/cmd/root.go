package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"gids/internal/logger"
)

// NewRootCommand returns a fresh root command tree.
// Using a constructor (rather than a package-level var) lets each test
// get its own command instance, avoiding flag re-registration panics.
func NewRootCommand() *cobra.Command {
	var verbose bool

	root := &cobra.Command{
		Use:   "gids",
		Short: "Git Identity Swap - manage multiple Git identities",
		Long: `gids (Git Identity Swap) manages multiple Git identities on one machine.
Store profiles (name, email, SSH key) and map directory globs to them as rules.
The shell hook applies the matching profile automatically when you cd.`,
		Example: `  # Add a new identity profile
  gids profile add

  # Apply the "work" profile to the current git repo
  gids use work

  # See which profile is active and why
  gids status

  # Set up auto-switching so profiles apply when you cd
  gids hook zsh >> ~/.zshrc`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			l := logger.New(os.Stderr, verbose)
			cmd.SetContext(logger.WithContext(cmd.Context(), l))
			return nil
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	root.AddCommand(newVersionCommand())
	root.AddCommand(newProfileCmd())
	root.AddCommand(newUseCmd())
	root.AddCommand(newRuleCmd())
	root.AddCommand(newHookCmd())
	root.AddCommand(newCheckCmd())

	return root
}

// Execute builds the default command tree and runs it.
func Execute() error {
	return NewRootCommand().Execute()
}
