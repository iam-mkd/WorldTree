// Package ports finds free host ports for worlds.
//
// A port counts as free only if it can be bound on every address Docker might
// publish it on: 127.0.0.1, ::1, 0.0.0.0 and ::, for both TCP and UDP. Spike A
// found a native Postgres on 127.0.0.1:5432 that Docker's IPv6 bind did not
// notice, so probing one address is not enough.
package ports

import (
	"errors"
	"net"
	"strconv"
)

// probeAddrs are the addresses a port must be bindable on.
var probeAddrs = []string{"127.0.0.1", "::1", "0.0.0.0", "::"}

// Free reports whether port can be bound on every probe address, TCP and UDP.
// An address family the host does not have (no IPv6) is skipped, not counted as busy.
// Anything else, including permission errors (Windows excluded port ranges), counts as busy.
func Free(port int) bool {
	p := strconv.Itoa(port)
	for _, a := range probeAddrs {
		hp := net.JoinHostPort(a, p)
		l, err := net.Listen("tcp", hp)
		if err != nil {
			if unavailable(err) {
				continue
			}
			return false
		}
		u, uerr := net.ListenPacket("udp", hp)
		_ = l.Close()
		if uerr != nil {
			if unavailable(uerr) {
				continue
			}
			return false
		}
		_ = u.Close()
	}
	return true
}

// unavailable reports errors meaning "this address does not exist here".
func unavailable(err error) bool {
	for _, e := range unavailableErrnos {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
