package registry

import (
	"fmt"
	"time"

	"github.com/iam-mkd/WorldTree/internal/ports"
)

// Put inserts or updates a world, keeping its CreatedAt.
func (s *State) Put(w World, now time.Time) {
	if old, ok := s.Worlds[w.ID]; ok {
		w.CreatedAt = old.CreatedAt
	} else {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	s.Worlds[w.ID] = &w
}

// Remove deletes a world and releases all of its leases. Removing an unknown
// world is not an error (idempotent `down` / `gc`).
func (s *State) Remove(id string) {
	delete(s.Worlds, id)
	kept := s.Leases[:0]
	for _, l := range s.Leases {
		if l.World != id {
			kept = append(kept, l)
		}
	}
	s.Leases = kept
}

// LeasesOf returns the world's leases as key -> port.
func (s *State) LeasesOf(id string) map[string]int {
	m := map[string]int{}
	for _, l := range s.Leases {
		if l.World == id {
			m[l.Key] = l.Port
		}
	}
	return m
}

// Lease makes the world's leases exactly keys: existing leases with a requested key
// are kept (same port as last time, not re-probed: the world's own containers may
// hold them), new keys get fresh ports from alloc, and leases for keys no longer
// requested are released. Returns key -> port for every key.
func (s *State) Lease(id string, keys []string, alloc ports.Allocator) (map[string]int, error) {
	if _, ok := s.Worlds[id]; !ok {
		return nil, fmt.Errorf("registry: lease for unregistered world %s", id)
	}
	want := map[string]bool{}
	for _, k := range keys {
		if want[k] {
			return nil, fmt.Errorf("registry: duplicate lease key %q", k)
		}
		want[k] = true
	}
	have := s.LeasesOf(id)
	out := map[string]int{}
	var missing []string
	for _, k := range keys {
		if p, ok := have[k]; ok {
			out[k] = p
		} else {
			missing = append(missing, k)
		}
	}
	// Drop stale leases of this world before picking, so their ports can be reused.
	kept := s.Leases[:0]
	taken := map[int]bool{}
	for _, l := range s.Leases {
		if l.World == id && !want[l.Key] {
			continue
		}
		kept = append(kept, l)
		taken[l.Port] = true
	}
	s.Leases = kept
	if len(missing) == 0 {
		return out, nil
	}
	picked, err := alloc.Pick(id, len(missing), taken)
	if err != nil {
		return nil, err
	}
	for i, k := range missing {
		out[k] = picked[i]
		s.Leases = append(s.Leases, Lease{Port: picked[i], World: id, Key: k})
	}
	return out, nil
}
