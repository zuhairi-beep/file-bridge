package main

import "errors"

// ErrAlreadyRunning is returned when another File Bridge process holds the lock.
var ErrAlreadyRunning = errors.New("file bridge is already running")

// EnsureSingleInstance returns an error if another instance is active.
// Call release() on shutdown (optional; OS releases on process exit).
func EnsureSingleInstance() (release func(), err error) {
	return platformEnsureSingleInstance()
}
