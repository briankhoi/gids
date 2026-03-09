package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"gids/internal/version"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the version of gids",
		Example: "  gids version",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "gids version %s\n", version.Get())
			return nil
		},
	}
}
