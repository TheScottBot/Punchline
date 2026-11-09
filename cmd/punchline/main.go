// Command punchline is Disciplinary Close-Up: keyboard-triggered webcam zoom
// from the tray. This file only wires the packages together.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/desktop"
	"github.com/thescottbot/punchline/internal/eventlog"
	"github.com/thescottbot/punchline/internal/product"
	"github.com/thescottbot/punchline/internal/settings"
)

var version = "dev"

func main() {
	quit := flag.Bool("quit", false, "ask the running instance to quit, then exit")
	flag.Parse()
	if *quit {
		if !desktop.RequestQuit() {
			os.Exit(1)
		}
		return
	}

	dir, err := settings.Dir()
	if err != nil {
		fatal(err)
	}
	lf, err := eventlog.Open(filepath.Join(dir, product.ExeName+".log"), eventlog.DefaultMax)
	if err != nil {
		fatal(err)
	}
	defer lf.Close()
	lg := log.New(lf, "", log.LstdFlags|log.Lmicroseconds)

	shell, err := desktop.New(desktop.Config{Logger: lg, SettingsDir: dir})
	if errors.Is(err, desktop.ErrAlreadyRunning) {
		lg.Printf("second launch: already running")
		return
	}
	if err != nil {
		lg.Printf("desktop: %v", err)
		fatal(err)
	}
	lg.Printf("%s %s starting", product.DisplayName, version)

	path := filepath.Join(dir, settings.FileName)
	s, problems, err := settings.Load(path)
	if err != nil {
		problems = append(problems, "could not read settings: "+err.Error())
	}
	for _, p := range problems {
		lg.Printf("settings: %s", p)
	}

	core := app.New(app.Options{
		Adapters:     adapters(),
		Registrar:    shell.Registrar(),
		Store:        settings.FileStore(path),
		Settings:     s,
		Logger:       lg,
		OnChange:     shell.Changed,
		PollInterval: 10 * time.Second,
	})
	if err := shell.Run(core, problems); err != nil {
		lg.Printf("desktop: %v", err)
		fatal(err)
	}
	lg.Printf("exited")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, product.ExeName+":", err)
	os.Exit(1)
}
