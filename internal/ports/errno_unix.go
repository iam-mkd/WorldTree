//go:build !windows

package ports

import "syscall"

var unavailableErrnos = []error{syscall.EADDRNOTAVAIL, syscall.EAFNOSUPPORT, syscall.EPROTONOSUPPORT}
