package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/tomaslatall/nowplay/internal/history"
	"github.com/tomaslatall/nowplay/internal/source"
	"github.com/tomaslatall/nowplay/internal/ui"
	"github.com/tomaslatall/nowplay/internal/volume"
)

func main() {
	demo := flag.Bool("demo", false, "run with scripted fake data")
	bgFlag := flag.String("background", "", "terminal background colour as #rrggbb (default: ask the terminal)")
	flag.Parse()

	bg, err := background(*bgFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nowplay:", err)
		os.Exit(2)
	}

	// SIGHUP is what a closed terminal or tuios pane sends; Go's default would
	// skip all cleanup and leave media-control running.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	defer cancel()

	var src source.Source = source.NewFake()
	recent := history.Demo()
	if !*demo {
		mc := source.NewMediaControl()
		if !mc.Installed() {
			fmt.Fprintln(os.Stderr, "nowplay needs media-control:\n  brew tap ungive/media-control && brew install media-control")
			os.Exit(1)
		}
		src, recent = mc, nil // recent is loaded from disk below
	}
	deps := ui.Deps{Control: src.Control}
	if *demo {
		deps.Volume = volume.NewFake(55)
	} else {
		if volume.Available() {
			deps.Volume = volume.New()
		}
		if path, err := history.DefaultPath(); err == nil {
			deps.History = history.NewStore(path, history.DefaultSize)
			var lerr error
			if recent, lerr = deps.History.Load(); lerr != nil {
				fmt.Fprintln(os.Stderr, "nowplay:", lerr)
			}
		}
	}

	states := make(chan source.State, 1)
	stopped := make(chan struct{})
	go func() {
		src.Run(ctx, states)
		close(stopped)
	}()

	p := tea.NewProgram(ui.New(states, recent, bg, deps), tea.WithAltScreen())
	go func() {
		<-ctx.Done()
		p.Quit()
	}()
	_, err = p.Run()

	// Let the source reap its child process before we exit.
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nowplay:", err)
		os.Exit(1)
	}
}

// background resolves the terminal background colour: the flag if given,
// otherwise the terminal's own answer (termenv also falls back to COLORFGBG
// and then black).
func background(flagValue string) (color.RGBA, error) {
	if flagValue != "" {
		return parseHex(flagValue)
	}
	// Blocks up to 5 s on terminals that never reply (OSCTimeout is a const);
	// --background avoids the query.
	r, g, b := termenv.ConvertToRGB(termenv.BackgroundColor()).RGB255()
	return color.RGBA{r, g, b, 255}, nil
}

func parseHex(s string) (color.RGBA, error) {
	h := s
	if len(h) > 0 && h[0] == '#' {
		h = h[1:]
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil || len(h) != 6 {
		return color.RGBA{}, fmt.Errorf("--background %q: want #rrggbb", s)
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}
