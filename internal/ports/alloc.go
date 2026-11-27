package ports

import (
	"fmt"
	"hash/fnv"
)

// Default range: above most dev servers and databases, below Linux's ephemeral range (32768+).
const (
	DefaultMin = 20000
	DefaultMax = 29999
)

// Allocator picks free ports in [Min, Max].
type Allocator struct {
	Min, Max int
	// Probe reports whether a port is free on the host. Nil means Free.
	Probe func(port int) bool
}

// Default returns the allocator for the default range with the real probe.
func Default() Allocator { return Allocator{Min: DefaultMin, Max: DefaultMax} }

// Pick returns n ports that are not in taken and pass the probe. The search starts at
// an offset derived from seed (the world id), so a world tends to get the same block
// again after it is removed and recreated, and different worlds start far apart.
// It scans the whole range once before giving up.
func (a Allocator) Pick(seed string, n int, taken map[int]bool) ([]int, error) {
	if a.Min <= 0 || a.Max < a.Min || a.Max > 65535 {
		return nil, fmt.Errorf("invalid port range %d-%d", a.Min, a.Max)
	}
	probe := a.Probe
	if probe == nil {
		probe = Free
	}
	size := a.Max - a.Min + 1
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	start := int(h.Sum32() % uint32(size))

	var out []int
	for i := 0; i < size && len(out) < n; i++ {
		p := a.Min + (start+i)%size
		if taken[p] || !probe(p) {
			continue
		}
		out = append(out, p)
	}
	if len(out) < n {
		return nil, fmt.Errorf("port range %d-%d exhausted: need %d free ports, found %d", a.Min, a.Max, n, len(out))
	}
	return out, nil
}
