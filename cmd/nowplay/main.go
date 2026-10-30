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

	if !*demo {
		fmt.Fprintln(os.Stderr, "nowplay: the media-control source arrives in milestone 2; use --demo for now")
		os.Exit(2)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var src source.Source = source.NewFake()
	states := make(chan source.State, 1)
	go src.Run(ctx, states)

	p := tea.NewProgram(ui.New(states, history.Demo()), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "nowplay:", err)
		os.Exit(1)
	}
}
