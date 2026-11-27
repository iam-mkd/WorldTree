package ports

import (
	"net"
	"testing"
)

// hold binds network/addr on an OS-chosen port and returns the port.
func hold(t *testing.T, network, addr string) int {
	t.Helper()
	switch network {
	case "udp":
		c, err := net.ListenPacket("udp", net.JoinHostPort(addr, "0"))
		if err != nil {
			t.Skipf("cannot bind %s %s: %v", network, addr, err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c.LocalAddr().(*net.UDPAddr).Port
	default:
		l, err := net.Listen("tcp", net.JoinHostPort(addr, "0"))
		if err != nil {
			t.Skipf("cannot bind %s %s: %v", network, addr, err)
		}
		t.Cleanup(func() { _ = l.Close() })
		return l.Addr().(*net.TCPAddr).Port
	}
}

func TestFreeSeesEveryAddress(t *testing.T) {
	for _, c := range []struct{ network, addr string }{
		{"tcp", "127.0.0.1"}, {"tcp", "::1"}, {"tcp", "0.0.0.0"}, {"tcp", "::"}, {"udp", "127.0.0.1"},
	} {
		t.Run(c.network+"/"+c.addr, func(t *testing.T) {
			p := hold(t, c.network, c.addr)
			if Free(p) {
				t.Errorf("port %d held on %s %s but Free says free", p, c.network, c.addr)
			}
		})
	}
}

// An OS-chosen TCP port on 127.0.0.1 may still be in use on another address or for
// UDP by an unrelated process, so try a few candidates and fail only if none is free.
func TestFreeOnUnusedPort(t *testing.T) {
	const attempts = 20
	var tried []int
	for i := 0; i < attempts; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		p := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if Free(p) {
			return
		}
		tried = append(tried, p)
	}
	t.Errorf("Free said busy for all %d just-released ports: %v", attempts, tried)
}

func TestPickSkipsTakenAndBusy(t *testing.T) {
	busy := map[int]bool{100: true, 102: true}
	a := Allocator{Min: 100, Max: 105, Probe: func(p int) bool { return !busy[p] }}
	got, err := a.Pick("", 3, map[int]bool{101: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if p == 100 || p == 101 || p == 102 {
			t.Errorf("picked unavailable port %d (%v)", p, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %v, want 3 ports", got)
	}
}

func TestPickSkipsRealHeldPort(t *testing.T) {
	p := hold(t, "tcp", "127.0.0.1")
	a := Allocator{Min: p, Max: p + 1}
	got, err := a.Pick("x", 1, nil)
	if err != nil {
		t.Skipf("neighbour port also busy: %v", err)
	}
	if got[0] == p {
		t.Errorf("picked held port %d", p)
	}
}

func TestPickExhausted(t *testing.T) {
	a := Allocator{Min: 100, Max: 102, Probe: func(int) bool { return true }}
	if _, err := a.Pick("", 4, nil); err == nil {
		t.Error("want exhausted error")
	}
	if _, err := (Allocator{Min: 10, Max: 5}).Pick("", 1, nil); err == nil {
		t.Error("want invalid range error")
	}
}

func TestPickIsDeterministicPerSeed(t *testing.T) {
	a := Allocator{Min: 20000, Max: 29999, Probe: func(int) bool { return true }}
	x, _ := a.Pick("world-a", 2, nil)
	y, _ := a.Pick("world-a", 2, nil)
	z, _ := a.Pick("world-b", 2, nil)
	if x[0] != y[0] || x[1] != x[0]+1 {
		t.Errorf("same seed should give same block: %v %v", x, y)
	}
	if x[0] == z[0] {
		t.Errorf("different seeds start at the same port %d", x[0])
	}
}
