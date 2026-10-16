package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	dataDir := flag.String("data-dir", "", "directory for profiles.json (no secrets)")
	flag.Parse()
	if *dataDir == "" {
		fmt.Fprintln(os.Stderr, "data-dir is required")
		os.Exit(2)
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "data-dir")
		os.Exit(1)
	}
	if err := serve(os.Stdin, os.Stdout, &App{dataDir: *dataDir}); err != nil {
		fmt.Fprintln(os.Stderr, "agent stopped")
		os.Exit(1)
	}
}
