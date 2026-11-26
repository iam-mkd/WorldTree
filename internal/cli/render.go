package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/iam-mkd/WorldTree/internal/compose"
	"github.com/iam-mkd/WorldTree/internal/world"
	"github.com/spf13/cobra"
)

func newRenderCmd() *cobra.Command {
	var portBase int
	cmd := &cobra.Command{
		Use:   "render [dir]",
		Short: "Print the world override and findings for a compose project, without touching Docker",
		Long: `render analyses the compose project in dir (default: current directory) and prints the
override worldtree would generate to stdout, and the ports and findings to stderr.

Ports are assigned sequentially from --port-base; the real allocator (with leases and
probing) is used by 'up'. Exit status is 1 if any finding is an error (the project
cannot be isolated as is).`,
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
			raw, err := compose.LoadConfig(ctx, abs)
			if err != nil {
				return err
			}
			cfg, err := compose.ParseConfig(raw)
			if err != nil {
				return err
			}
			id := world.ID(abs)
			plan, err := compose.NewPlan(cfg, compose.Options{
				WorldID:    id,
				Project:    world.ProjectName(filepath.Base(abs), id),
				ProjectDir: abs,
				IsFile:     isFile,
			})
			if err != nil {
				return err
			}
			leases := map[string]int{}
			for i, r := range plan.Ports {
				leases[r.Key] = portBase + i
			}
			out, err := plan.Render(leases)
			if err != nil {
				return err
			}
			if _, err := cmd.OutOrStdout().Write(out); err != nil {
				return err
			}
			printPlan(cmd, plan, leases)
			if plan.HasErrors() {
				return fmt.Errorf("compose project cannot be isolated as is (see error findings)")
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&portBase, "port-base", 20000, "first host port to assign")
	return cmd
}

func printPlan(cmd *cobra.Command, plan *compose.Plan, leases map[string]int) {
	w := cmd.ErrOrStderr()
	if len(plan.Ports) > 0 {
		fmt.Fprintln(w, "\nports:")
		for _, r := range plan.Ports {
			host := r.Port.HostIP
			if host == "" {
				host = "*"
			}
			fmt.Fprintf(w, "  %-20s %s:%s -> %d  (container %d/%s)\n", r.Service, host, r.Port.Published, leases[r.Key], r.Port.Target, r.Port.Protocol)
		}
	}
	if len(plan.Findings) > 0 {
		fs := append([]compose.Finding(nil), plan.Findings...)
		sort.SliceStable(fs, func(i, j int) bool { return fs[i].Severity > fs[j].Severity })
		fmt.Fprintln(w, "\nfindings:")
		for _, f := range fs {
			fmt.Fprintf(w, "  %s\n", f)
		}
	}
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
