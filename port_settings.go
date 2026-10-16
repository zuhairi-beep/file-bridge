package main

import (
	"errors"
	"fmt"
	"net"
)

const minUserPort = 1024
const maxUserPort = 65535
const highPortScanStart = 49152

// PortStartError is returned when the HTTP server cannot bind the configured port.
type PortStartError struct {
	Port      int
	Busy      bool
	FileBridge bool
	Err       error
}

func (e *PortStartError) Error() string {
	if e == nil {
		return ""
	}
	if e.FileBridge {
		return fmt.Sprintf("another File Bridge is already listening on port %d", e.Port)
	}
	if e.Busy {
		return fmt.Sprintf("port %d is in use by another program", e.Port)
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("could not start on port %d", e.Port)
}

func configuredPort(cfg *Config) int {
	if cfg == nil || cfg.Port == 0 {
		return defaultPort
	}
	return cfg.Port
}

func validatePort(port int) error {
	if port < minUserPort || port > maxUserPort {
		return fmt.Errorf("port must be between %d and %d", minUserPort, maxUserPort)
	}
	return nil
}

// isLocalPortFree returns true if we can bind TCP on the given port right now.
func isLocalPortFree(port int) bool {
	if port == 0 {
		port = 1
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// findFreeHighPort picks the first free port from highPortScanStart upward.
func findFreeHighPort() (int, error) {
	for p := highPortScanStart; p <= maxUserPort; p++ {
		if isLocalPortFree(p) {
			return p, nil
		}
	}
	return 0, errors.New("no free port found in the high port range")
}
