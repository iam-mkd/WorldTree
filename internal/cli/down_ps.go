package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"text/tabwriter"

	"github.com/iam-mkd/WorldTree/internal/registry"
	"github.com/iam-mkd/WorldTree/internal/world"
	"github.com/spf13/cobra"
)

func openRegistry() (*registry.Registry, error) {
	dir, err := registry.DefaultDir()
	if err != nil {
		return nil, err
	}
	return registry.Open(dir)
}

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [dir]",
		Short: "Remove a world: containers, networks, volumes (incl. its database), ports and generated files",
		Long: `down removes the world of dir (default: current directory): its containers, networks and
named volumes, including the world's database, then its port leases, registry record and
<dir>/.worldtree. Your compose files are never touched.

dir does not have to exist any more: 'worldtree down <path>' cleans up after a deleted worktree.
down is idempotent: running it on an unknown or half-created world finishes the job.`,
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
			reg, err := openRegistry()
			if err != nil {
				return err
			}
			res, err := world.NewDowner(reg, cmd.ErrOrStderr()).Down(cmd.Context(), abs)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !res.Registered {
				fmt.Fprintf(out, "world %s was not registered; removed any leftovers of project %s\n", res.ID, res.Project)
			} else {
				fmt.Fprintf(out, "world %s removed: project %s\n", res.ID, res.Project)
			}
			if res.Swept > 0 {
				fmt.Fprintf(out, "  swept %d labelled resource(s) compose down left behind\n", res.Swept)
			}
			return nil
		},
	}
}

func newPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List worlds with their phase, containers, ports and directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := openRegistry()
			if err != nil {
				return err
			}
			worlds, err := world.List(cmd.Context(), reg, dockerOutput)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(worlds) == 0 {
				fmt.Fprintln(out, "no worlds")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tPHASE\tCONTAINERS\tPORTS\tDIR")
			missing := 0
			for _, w := range worlds {
				containers := "?"
				if w.DockerOK {
					containers = fmt.Sprintf("%d/%d running", w.Running, w.Containers)
				}
				dir := w.Dir
				if w.DirMissing {
					dir += "  (dir missing)"
					missing++
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", w.ID, w.Phase, containers, world.FormatPorts(w.Ports), dir)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if missing > 0 {
				// No ready-made command: quoting rules differ between shells (sh, PowerShell).
				fmt.Fprintf(out, "\n%d world(s) whose directory is gone; remove each with `worldtree down <DIR>` (quote the path).\n", missing)
			}
			return nil
		},
	}
}

func dockerOutput(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).Output()
}
