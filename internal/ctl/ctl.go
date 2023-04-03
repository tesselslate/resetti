// Package ctl implements the main controller used for all of the available
// resetting schemes (e.g. multi, wall.)
package ctl

// bufferSize is the capacity a buffered channel that processes per-instance
// state should have for each instance.
const bufferSize = 16
