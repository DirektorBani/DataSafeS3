package syncapp

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchOptions configures background sync.
type WatchOptions struct {
	Options
	Interval   time.Duration
	UseFSNotify bool
	OnResult   func(SyncResult)
	OnError    func(error)
}

// Watch runs sync in a loop until ctx is cancelled.
func Watch(ctx context.Context, opts WatchOptions) error {
	if opts.Interval <= 0 {
		opts.Interval = 30 * time.Second
	}
	run := func() {
		res, err := RunOnce(opts.Options)
		if err != nil {
			if opts.OnError != nil {
				opts.OnError(err)
			}
			return
		}
		if opts.OnResult != nil {
			opts.OnResult(res)
		}
	}

	if !opts.UseFSNotify {
		ticker := time.NewTicker(opts.Interval)
		defer ticker.Stop()
		run()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				run()
			}
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}
	defer watcher.Close()

	if err := addWatchRecursive(watcher, opts.Folder); err != nil {
		return err
	}

	debounce := time.NewTimer(opts.Interval)
	debounce.Stop()

	run()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				if filepath.Base(event.Name) == conflictsDirName {
					continue
				}
				debounce.Reset(opts.Interval)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			if opts.OnError != nil {
				opts.OnError(err)
			}
		case <-debounce.C:
			run()
		}
	}
}

func addWatchRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == conflictsDirName {
				return filepath.SkipDir
			}
			if err := w.Add(path); err != nil {
				return err
			}
		}
		return nil
	})
}
