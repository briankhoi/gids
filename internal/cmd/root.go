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
		Use:           "gids",
		Short:         "Git Identity Swap - manage multiple Git identities",
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

	return root
}

// Execute builds the default command tree and runs it.
func Execute() error {
	return NewRootCommand().Execute()
}
