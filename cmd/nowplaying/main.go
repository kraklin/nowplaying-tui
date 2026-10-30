// Command nowplaying is a pixel-art now-playing card for the terminal (macOS).
package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/tomaslatall/nowplaying-tui/internal/config"
	"github.com/tomaslatall/nowplaying-tui/internal/history"
	"github.com/tomaslatall/nowplaying-tui/internal/source"
	"github.com/tomaslatall/nowplaying-tui/internal/ui"
	"github.com/tomaslatall/nowplaying-tui/internal/volume"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3". Without
// it the module version from the build info is used (go install ...@v1.2.3).
var version = ""

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func main() { os.Exit(run()) }

// run is main with an exit code, so deferred cleanup runs before the exit.
func run() int {
	demo := flag.Bool("demo", false, "run with scripted fake data")
	bgFlag := flag.String("background", "", "terminal background colour as #rrggbb (default: config, then ask the terminal)")
	cfgFlag := flag.String("config", "", "config file (default: $XDG_CONFIG_HOME/nowplaying/config.toml)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("nowplaying", versionString())
		return 0
	}

	cfg, warnings := loadConfig(*cfgFlag)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "nowplaying:", w)
	}

	bg, err := background(*bgFlag, cfg.Background)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nowplaying:", err)
		return 2
	}

	// SIGHUP is what a closed terminal or tuios pane sends; Go's default would
	// skip all cleanup and leave media-control running.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	defer cancel()

	src, deps, recent := wire(*demo, cfg, warnings)
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
		fmt.Fprintln(os.Stderr, "nowplaying:", err)
		return 1
	}
	return 0
}

// wire picks the source, volume control and history store: scripted fakes for
// --demo, the real macOS ones otherwise. Without media-control the source
// reports it and the UI shows the install hint.
func wire(demo bool, cfg config.Config, warnings []string) (source.Source, ui.Deps, []history.Entry) {
	deps := ui.Deps{Config: &cfg, Warnings: warnings}
	if demo {
		src := source.NewFake()
		deps.Control, deps.Volume = src.Control, volume.NewFake(55)
		return src, deps, history.Demo()
	}
	src := source.NewMediaControl()
	deps.Control = src.Control
	if volume.Available() {
		deps.Volume = volume.New()
	}
	if !cfg.PersistHistory {
		return src, deps, nil // the list lives in memory for this session only
	}
	path, err := history.DefaultPath()
	if err != nil {
		return src, deps, nil
	}
	store := history.NewStore(path, cfg.HistorySize)
	if legacy, err := history.LegacyPath(); err == nil {
		store.Legacy = legacy
	}
	deps.History = store
	recent, err := store.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "nowplaying:", err)
		deps.Warnings = append(deps.Warnings, "history was unreadable; started empty")
	}
	return src, deps, recent
}

// loadConfig reads the file named by --config, or the default location. A
// file named explicitly must exist; a missing default file is simply the defaults.
func loadConfig(flagPath string) (config.Config, []string) {
	path := flagPath
	if path == "" {
		var err error
		if path, err = config.DefaultFile(); err != nil {
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
