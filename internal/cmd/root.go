package cmd

import (
	"github.com/spf13/cobra"
)

// NewRootCommand returns a fresh root command tree.
// Using a constructor (rather than a package-level var) lets each test
// get its own command instance, avoiding flag re-registration panics.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "gids",
		Short:         "Git Identity Swap - manage multiple Git identities",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCommand())

	return root
}

// Execute builds the default command tree and runs it.
func Execute() error {
	return NewRootCommand().Execute()
}
