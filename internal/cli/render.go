package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/iam-mkd/WorldTree/internal/compose"
	"github.com/iam-mkd/WorldTree/internal/ports"
	"github.com/iam-mkd/WorldTree/internal/registry"
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

Ports come from the real allocator as a dry run: the world's existing leases are reused,
other worlds' leases and ports busy on the host are skipped, but nothing is saved, so a
later 'up' may pick differently if the host changes. --port-base skips the allocator.

Exit status is 1 if any finding is an error (the project cannot be isolated as is).`,
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
			a, err := world.Analyse(ctx, abs, compose.LoadConfig)
			if err != nil {
				return err
			}
			plan := a.Plan
			leases, err := dryRunLeases(plan, a.ID, abs, portBase)
			if err != nil {
				return err
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
	cmd.Flags().IntVar(&portBase, "port-base", 0, "assign ports sequentially from this port instead of asking the allocator (tests)")
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

// dryRunLeases computes the ports `up` would lease, on a copy of the registry state.
func dryRunLeases(plan *compose.Plan, id, dir string, base int) (map[string]int, error) {
	keys := make([]string, len(plan.Ports))
	for i, r := range plan.Ports {
		keys[i] = r.Key
	}
	if base > 0 {
		m := map[string]int{}
		for i, k := range keys {
			m[k] = base + i
		}
		return m, nil
	}
	regDir, err := registry.DefaultDir()
	if err != nil {
		return nil, err
	}
	reg, err := registry.Open(regDir)
	if err != nil {
		return nil, err
	}
	st, err := reg.Load() // unlocked snapshot; never saved
	if err != nil {
		return nil, err
	}
	if _, ok := st.Worlds[id]; !ok {
		st.Put(registry.World{ID: id, Dir: dir, Phase: registry.Creating}, time.Now())
	}
	return st.Lease(id, keys, ports.Default())
}
