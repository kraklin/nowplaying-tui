package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/tomaslatall/nowplay/internal/config"
	"github.com/tomaslatall/nowplay/internal/history"
	"github.com/tomaslatall/nowplay/internal/source"
	"github.com/tomaslatall/nowplay/internal/ui"
	"github.com/tomaslatall/nowplay/internal/volume"
)

func main() {
	demo := flag.Bool("demo", false, "run with scripted fake data")
	bgFlag := flag.String("background", "", "terminal background colour as #rrggbb (default: config, then ask the terminal)")
	cfgFlag := flag.String("config", "", "config file (default: $XDG_CONFIG_HOME/nowplay/config.toml)")
	flag.Parse()

	cfg, warnings := loadConfig(*cfgFlag)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "nowplay:", w)
	}

	bg, err := background(*bgFlag, cfg.Background)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nowplay:", err)
		os.Exit(2)
	}

	// SIGHUP is what a closed terminal or tuios pane sends; Go's default would
	// skip all cleanup and leave media-control running.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	defer cancel()

	// Without media-control the source reports it and the UI shows the install hint.
	var src source.Source = source.NewFake()
	recent := history.Demo()
	if !*demo {
		src, recent = source.NewMediaControl(), nil // recent is loaded from disk below
	}
	deps := ui.Deps{Control: src.Control, Config: &cfg, Warnings: warnings}
	if *demo {
		deps.Volume = volume.NewFake(55)
	} else {
		if volume.Available() {
			deps.Volume = volume.New()
		}
		if path, err := history.DefaultPath(); err == nil {
			deps.History = history.NewStore(path, cfg.HistorySize)
			var lerr error
			if recent, lerr = deps.History.Load(); lerr != nil {
				fmt.Fprintln(os.Stderr, "nowplay:", lerr)
				deps.Warnings = append(deps.Warnings, "history was unreadable; started empty")
			}
		}
	}

	states := make(chan source.State, 1)
	stopped := make(chan struct{})
	go func() {
		src.Run(ctx, states)
		close(stopped)
	}()

	p := tea.NewProgram(ui.New(states, recent, bg, deps), tea.WithAltScreen(), tea.WithMouseCellMotion())
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

// loadConfig reads the file named by --config, or the default location. A
// file named explicitly must exist; a missing default file is simply the defaults.
func loadConfig(flagPath string) (config.Config, []string) {
	path := flagPath
	if path == "" {
		var err error
		if path, err = config.Path(); err != nil {
			return config.Default(), nil
		}
	} else if _, err := os.Stat(path); err != nil {
		return config.Default(), []string{"config: cannot read " + path}
	}
	return config.Load(path)
}

// background resolves the terminal background colour: the flag, then the
// config file, then the terminal's own answer (termenv also falls back to
// COLORFGBG and then black).
func background(flagValue, configValue string) (color.RGBA, error) {
	switch {
	case flagValue != "":
		c, err := config.ParseHex(flagValue)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("--background %w", err)
		}
		return c, nil
	case configValue != "":
		return config.ParseHex(configValue) // already validated by config.Load
	}
	// Blocks up to 5 s on terminals that never reply (OSCTimeout is a const);
	// the flag or the config key avoids the query.
	r, g, b := termenv.ConvertToRGB(termenv.BackgroundColor()).RGB255()
	return color.RGBA{r, g, b, 255}, nil
}
