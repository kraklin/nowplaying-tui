package source

import "context"

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
