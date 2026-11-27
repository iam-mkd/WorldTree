//go:build windows

package ports

import "syscall"

// WSAEADDRNOTAVAIL, WSAEAFNOSUPPORT, WSAEPROTONOSUPPORT.
var unavailableErrnos = []error{syscall.Errno(10049), syscall.Errno(10047), syscall.Errno(10043)}
