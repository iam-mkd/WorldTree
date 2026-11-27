package cli

import (
	"fmt"
	"path/filepath"

	"github.com/iam-mkd/WorldTree/internal/compose"
	"github.com/iam-mkd/WorldTree/internal/registry"
	"github.com/iam-mkd/WorldTree/internal/world"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up [dir]",
		Short: "Create or reconcile the isolated world for a compose project and wait until it is healthy",
		Long: `up turns the compose project in dir (default: current directory) into an isolated world:
its own compose project name, leased host ports, no fixed container names, labelled resources.

The original compose files are never modified; the world's override is written to
<dir>/.worldtree/compose.override.yml. up is idempotent: re-running it after a failure or
crash continues from what exists and keeps the same ports. Compose output goes to stderr;
the world summary goes to stdout.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if _, err := compose.CheckVersion(ctx); err != nil {
				return err
			}
			regDir, err := registry.DefaultDir()
			if err != nil {
				return err
			}
			reg, err := registry.Open(regDir)
			if err != nil {
				return err
			}
			res, err := world.NewUpper(reg, cmd.ErrOrStderr()).Up(ctx, abs)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "world %s ready: project %s\n", res.ID, res.Project)
			for _, r := range res.Plan.Ports {
				host := r.Port.HostIP
				if host == "" {
					host = "*"
				}
				fmt.Fprintf(out, "  %-20s %s:%d -> container %d/%s\n", r.Service, host, res.Leases[r.Key], r.Port.Target, r.Port.Protocol)
			}
			return nil
		},
	}
}
