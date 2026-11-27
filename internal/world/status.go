package world

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/iam-mkd/WorldTree/internal/compose"
	"github.com/iam-mkd/WorldTree/internal/registry"
)

// Status is one world as `ps` shows it.
type Status struct {
	registry.World
	Ports      []PortStatus
	Running    int  // containers of the world in state running
	Containers int  // all containers of the world
	DockerOK   bool // false if docker could not be queried; Running/Containers unknown
	DirMissing bool // the world dir no longer exists (orphan: `worldtree down <dir>` or gc)
}

// PortStatus is one leased host port.
type PortStatus struct {
	Service string
	Port    int
}

// List returns every registered world, sorted by dir, with its leases and container
// counts. query runs `docker <args>`; if it fails, worlds are still listed with
// DockerOK false.
func List(ctx context.Context, reg *registry.Registry, query func(ctx context.Context, args ...string) ([]byte, error)) ([]Status, error) {
	st, err := reg.Load()
	if err != nil {
		return nil, err
	}
	running, total, dockerErr := containerCounts(ctx, query)
	var out []Status
	for _, w := range st.Worlds {
		s := Status{World: *w, DockerOK: dockerErr == nil}
		if s.DockerOK {
			s.Running, s.Containers = running[w.ID], total[w.ID]
		}
		if _, err := os.Stat(w.Dir); os.IsNotExist(err) {
			s.DirMissing = true
		}
		for key, port := range st.LeasesOf(w.ID) {
			svc, _, _ := strings.Cut(key, "/")
			s.Ports = append(s.Ports, PortStatus{Service: svc, Port: port})
		}
		sort.Slice(s.Ports, func(i, j int) bool { return s.Ports[i].Port < s.Ports[j].Port })
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}

// containerCounts returns, per world id, running and total containers, from one docker call.
func containerCounts(ctx context.Context, query func(ctx context.Context, args ...string) ([]byte, error)) (map[string]int, map[string]int, error) {
	out, err := query(ctx, "ps", "-a", "--filter", "label="+compose.LabelWorld,
		"--format", `{{.Label "`+compose.LabelWorld+`"}} {{.State}}`)
	if err != nil {
		return nil, nil, err
	}
	running, total := map[string]int{}, map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		id, state, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		total[id]++
		if state == "running" {
			running[id]++
		}
	}
	return running, total, nil
}

// FormatPorts renders ports as "app:21000 postgres:21001".
func FormatPorts(ps []PortStatus) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.Service + ":" + strconv.Itoa(p.Port)
	}
	return strings.Join(parts, " ")
}
