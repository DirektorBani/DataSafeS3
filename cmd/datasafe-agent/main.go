package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dataDir := flag.String("data-dir", "", "directory for profiles.json (no secrets)")
	purge := flag.Bool("purge-secrets", false, "delete Windows credentials whose target ends in .datasafe-desktop")
	flag.Parse()
	if *purge {
		n, err := purgeDesktopSecrets()
		if err != nil {
			fmt.Fprintln(os.Stderr, "purge-secrets")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "removed %d\n", n)
		return
	}
	if *dataDir == "" {
		fmt.Fprintln(os.Stderr, "data-dir is required")
		os.Exit(2)
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "data-dir")
		os.Exit(1)
	}
	lock, err := acquireAgentLock(*dataDir)
	if err != nil {
		if errors.Is(err, errAgentBusy) {
			fmt.Fprintln(os.Stderr, errAgentBusy.Error())
		} else {
			fmt.Fprintln(os.Stderr, "data-dir")
		}
		os.Exit(1)
	}
	defer releaseAgentLock(lock)
	logger, err := openLogger(filepath.Join(*dataDir, "logs"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "log")
		os.Exit(1)
	}
	app := &App{dataDir: *dataDir, log: logger}
	err = serve(os.Stdin, os.Stdout, app)
	if app.transfers != nil && app.transfers.journal != nil {
		_ = app.transfers.journal.Flush()
	}
	_ = logger.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent stopped")
		os.Exit(1)
	}
}
