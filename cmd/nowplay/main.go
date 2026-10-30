package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tomaslatall/nowplay/internal/history"
	"github.com/tomaslatall/nowplay/internal/source"
	"github.com/tomaslatall/nowplay/internal/ui"
)

func main() {
	demo := flag.Bool("demo", false, "run with scripted fake data")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var src source.Source = source.NewFake()
	recent := history.Demo()
	if !*demo {
		mc := source.NewMediaControl()
		if !mc.Installed() {
			fmt.Fprintln(os.Stderr, "nowplay needs media-control:\n  brew tap ungive/media-control && brew install media-control")
			os.Exit(1)
		}
		src, recent = mc, nil
	}
	states := make(chan source.State, 1)
	go src.Run(ctx, states)

	p := tea.NewProgram(ui.New(states, recent), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "nowplay:", err)
		os.Exit(1)
	}
}
