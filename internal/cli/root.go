// Package cli wires the worldtree commands.
package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Set at build time via -ldflags "-X github.com/iam-mkd/WorldTree/internal/cli.version=...".
var (
	version = "dev"
	commit  = "none"
)

// NewRootCmd returns the root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "worldtree",
		Short:         "Isolated local runtime worlds (compose, ports, Postgres, Redis) per git worktree",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newVersionCmd(), newRenderCmd(), newUpCmd(), newDownCmd(), newPsCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the worldtree version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "worldtree %s (%s) %s/%s\n", version, commit, runtime.GOOS, runtime.GOARCH)
		},
	}
}
