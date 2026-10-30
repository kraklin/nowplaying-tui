// Package source reports what the system is playing and runs transport
// commands: the media-control stream parser and a scripted fake for --demo.
package source

import "context"

// CmdKind is the kind of transport command.
type CmdKind int

const (
	TogglePlay CmdKind = iota
	Next
	Previous
	Seek
)

// Cmd is a transport command. Seconds is the absolute position for Seek.
type Cmd struct {
	Kind    CmdKind
	Seconds float64
}

// Source reports now-playing state and accepts transport commands.
type Source interface {
	// Run sends a State on every change until ctx is cancelled.
	Run(ctx context.Context, out chan<- State)
	Control(Cmd) error
}
