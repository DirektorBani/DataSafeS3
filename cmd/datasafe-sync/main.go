package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DirektorBani/datasafe/internal/syncapp"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "login":
		runLogin(os.Args[2:])
	case "sync":
		runSync(os.Args[2:])
	case "watch":
		runWatch(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "buckets":
		runBuckets(os.Args[2:])
	case "conflicts":
		runConflicts(os.Args[2:])
	case "resolve":
		runResolve(os.Args[2:])
	case "token":
		runToken(os.Args[2:])
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`DataSafeS3 Sync — desktop folder sync (Phase 3)

Usage:
  datasafe-sync login     --server URL --user NAME --password PASS [--profile default]
  datasafe-sync token set --token JWT [--profile default]
  datasafe-sync sync      --folder PATH [--bucket files] [--prefix ""] [--pull] [--push] [--delete]
                          [--conflict-policy last_write_wins|local_wins|remote_wins|keep_both]
                          [--json] [--json-lines] [--profile default]
  datasafe-sync watch     --folder PATH [--interval 30s] [--fsnotify] [--bucket files] [--prefix ""]
                          [--delete] [--conflict-policy ...] [--profile default]
  datasafe-sync status    [--json] [--profile default]
  datasafe-sync buckets   [--json] [--profile default]
  datasafe-sync conflicts [--folder PATH] [--profile default]
  datasafe-sync resolve   --name BACKUP_FILE [--folder PATH] [--profile default]

Sync uses the console REST API (JWT). Default bucket is the home bucket "files".
State: OS user config dir (~/.config/datasafe on Linux, %APPDATA% on Windows).

Examples:
  datasafe-sync login --server http://localhost:8080 --user alice --password secret
  datasafe-sync sync --folder ./DataSafeS3 --pull --push --delete
  datasafe-sync watch --folder ./DataSafeS3 --fsnotify --interval 5s
  datasafe-sync buckets --json
`)
}

func profileFlag(args []string) (string, []string) {
	fs := flag.NewFlagSet("cmd", flag.ExitOnError)
	profile := fs.String("profile", "default", "sync profile name")
	_ = fs.Parse(args)
	return *profile, fs.Args()
}

func loadClient(profile string) (*syncapp.Client, *syncapp.State, error) {
	st, err := syncapp.LoadState(profile)
	if err != nil {
		return nil, nil, err
	}
	if st.Profile.Token == "" {
		return nil, nil, fmt.Errorf("not logged in — run datasafe-sync login first")
	}
	return syncapp.NewClient(st.Profile.ServerURL, st.Profile.Token), st, nil
}

func runLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	server := fs.String("server", "http://localhost:8080", "console base URL")
	user := fs.String("user", "", "username")
	password := fs.String("password", "", "password")
	profile := fs.String("profile", "default", "profile name")
	_ = fs.Parse(args)
	if *user == "" || *password == "" {
		fail("login requires --user and --password")
	}
	c := syncapp.NewClient(*server, "")
	tok, err := c.Login(*user, *password)
	if err != nil {
		fail(err)
	}
	st, err := syncapp.LoadState(*profile)
	if err != nil {
		fail(err)
	}
	st.Profile.ServerURL = strings.TrimRight(*server, "/")
	st.Profile.Username = *user
	st.Profile.Token = tok
	if err := st.Save(*profile); err != nil {
		fail(err)
	}
	fmt.Printf("logged in as %s (profile %s)\n", *user, *profile)
}

func runToken(args []string) {
	if len(args) < 1 || args[0] != "set" {
		fail("usage: datasafe-sync token set --token JWT")
	}
	fs := flag.NewFlagSet("token set", flag.ExitOnError)
	tok := fs.String("token", "", "JWT from console")
	server := fs.String("server", "", "optional server URL")
	profile := fs.String("profile", "default", "profile name")
	_ = fs.Parse(args[1:])
	if *tok == "" {
		fail("token set requires --token")
	}
	st, err := syncapp.LoadState(*profile)
	if err != nil {
		fail(err)
	}
	st.Profile.Token = *tok
	if *server != "" {
		st.Profile.ServerURL = strings.TrimRight(*server, "/")
	}
	if err := st.Save(*profile); err != nil {
		fail(err)
	}
	fmt.Println("token saved")
}

func syncFlags(fs *flag.FlagSet) (*string, *string, *string, *bool, *bool, *bool, *string, *bool, *bool) {
	folder := fs.String("folder", "", "local folder path")
	bucket := fs.String("bucket", "files", "remote bucket")
	prefix := fs.String("prefix", "", "remote prefix inside bucket")
	pull := fs.Bool("pull", true, "download remote changes")
	push := fs.Bool("push", true, "upload local changes")
	deleteSync := fs.Bool("delete", false, "propagate deletions both ways")
	conflict := fs.String("conflict-policy", "last_write_wins", "conflict resolution policy")
	jsonOut := fs.Bool("json", false, "print result as JSON")
	jsonLines := fs.Bool("json-lines", false, "stream JSON-lines progress")
	return folder, bucket, prefix, pull, push, deleteSync, conflict, jsonOut, jsonLines
}

func runSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	folder, bucket, prefix, pull, push, deleteSync, conflict, jsonOut, jsonLines := syncFlags(fs)
	profile := fs.String("profile", "default", "profile name")
	_ = fs.Parse(args)
	if *folder == "" {
		fail("sync requires --folder")
	}
	c, st, err := loadClient(*profile)
	if err != nil {
		fail(err)
	}
	persistProfile(st, *profile, *folder, *bucket, *prefix)
	opts := syncapp.Options{
		ProfileName:    *profile,
		Client:         c,
		Folder:         *folder,
		Bucket:         *bucket,
		Prefix:         *prefix,
		Pull:           *pull,
		Push:           *push,
		DeleteSync:     *deleteSync,
		ConflictPolicy: syncapp.ParseConflictPolicy(*conflict),
	}
	if *jsonLines {
		res, err := syncapp.RunOnceJSON(opts, os.Stdout)
		if err != nil {
			os.Exit(1)
		}
		if len(res.Errors) > 0 {
			os.Exit(1)
		}
		return
	}
	res, err := syncapp.RunOnce(opts)
	if err != nil {
		fail(err)
	}
	if *jsonOut {
		raw, _ := res.JSON()
		fmt.Println(string(raw))
	} else {
		printResult(res)
	}
	if len(res.Errors) > 0 {
		os.Exit(1)
	}
}

func runWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	folder, bucket, prefix, pull, push, deleteSync, conflict, _, _ := syncFlags(fs)
	profile := fs.String("profile", "default", "profile name")
	interval := fs.Duration("interval", 30*time.Second, "poll or debounce interval")
	fsnotify := fs.Bool("fsnotify", false, "watch filesystem events (debounced)")
	_ = fs.Parse(args)
	if *folder == "" {
		fail("watch requires --folder")
	}
	c, st, err := loadClient(*profile)
	if err != nil {
		fail(err)
	}
	persistProfile(st, *profile, *folder, *bucket, *prefix)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := "poll"
	if *fsnotify {
		mode = "fsnotify"
	}
	fmt.Printf("watching %s ↔ %s/%s (%s, %s) — Ctrl+C to stop\n", *folder, *bucket, *prefix, mode, *interval)

	opts := syncapp.WatchOptions{
		Options: syncapp.Options{
			ProfileName:    *profile,
			Client:         c,
			Folder:         *folder,
			Bucket:         *bucket,
			Prefix:         *prefix,
			Pull:           *pull,
			Push:           *push,
			DeleteSync:     *deleteSync,
			ConflictPolicy: syncapp.ParseConflictPolicy(*conflict),
		},
		Interval:    *interval,
		UseFSNotify: *fsnotify,
		OnResult: func(res syncapp.SyncResult) {
			if res.Uploaded+res.Downloaded+res.DeletedLocal+res.DeletedRemote > 0 || len(res.Conflicts) > 0 {
				fmt.Printf("[%s] ", time.Now().Format(time.RFC3339))
				printResult(res)
			}
		},
		OnError: func(err error) {
			fmt.Fprintf(os.Stderr, "sync error: %v\n", err)
		},
	}
	if err := syncapp.Watch(ctx, opts); err != nil && err != context.Canceled {
		fail(err)
	}
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	profile := fs.String("profile", "default", "profile name")
	jsonOut := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)
	snap, err := syncapp.ProfileStatus(*profile)
	if err != nil {
		fail(err)
	}
	if *jsonOut {
		raw, _ := json.Marshal(snap)
		fmt.Println(string(raw))
		return
	}
	fmt.Printf("profile: %s\n", snap.ProfileName)
	fmt.Printf("server:  %s\n", snap.Profile.ServerURL)
	fmt.Printf("user:    %s\n", snap.Profile.Username)
	fmt.Printf("bucket:  %s\n", snap.Profile.Bucket)
	fmt.Printf("prefix:  %s\n", snap.Profile.Prefix)
	fmt.Printf("folder:  %s\n", snap.Profile.Folder)
	fmt.Printf("tracked: %d files\n", snap.Tracked)
	if snap.LoggedIn {
		fmt.Println("status:  logged in")
	} else {
		fmt.Println("status:  not logged in")
	}
}

func runBuckets(args []string) {
	fs := flag.NewFlagSet("buckets", flag.ExitOnError)
	profile := fs.String("profile", "default", "profile name")
	jsonOut := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)
	c, _, err := loadClient(*profile)
	if err != nil {
		fail(err)
	}
	buckets, err := c.ListBuckets()
	if err != nil {
		fail(err)
	}
	if *jsonOut {
		raw, _ := json.Marshal(map[string]any{"buckets": buckets})
		fmt.Println(string(raw))
		return
	}
	for _, b := range buckets {
		line := b.Name
		if b.Access != nil {
			line += fmt.Sprintf(" (%s", b.Access.Ownership)
			if b.Access.SharedBy != "" {
				line += ", shared by " + b.Access.SharedBy
			}
			line += ")"
		}
		fmt.Println(line)
	}
}

func runConflicts(args []string) {
	fs := flag.NewFlagSet("conflicts", flag.ExitOnError)
	folder := fs.String("folder", "", "local folder")
	profile := fs.String("profile", "default", "profile name")
	_ = fs.Parse(args)
	if *folder == "" {
		st, err := syncapp.LoadState(*profile)
		if err != nil {
			fail(err)
		}
		*folder = st.Profile.Folder
	}
	if *folder == "" {
		fail("conflicts requires --folder or saved profile folder")
	}
	names, err := syncapp.ListConflicts(*folder)
	if err != nil {
		fail(err)
	}
	if len(names) == 0 {
		fmt.Println("no conflict backups")
		return
	}
	for _, n := range names {
		fmt.Println(n)
	}
}

func runResolve(args []string) {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	name := fs.String("name", "", "conflict backup filename")
	folder := fs.String("folder", "", "local folder")
	profile := fs.String("profile", "default", "profile name")
	_ = fs.Parse(args)
	if *name == "" {
		fail("resolve requires --name")
	}
	if *folder == "" {
		st, err := syncapp.LoadState(*profile)
		if err != nil {
			fail(err)
		}
		*folder = st.Profile.Folder
	}
	if err := syncapp.ResolveConflict(*folder, *name); err != nil {
		fail(err)
	}
	fmt.Println("resolved:", *name)
}

func persistProfile(st *syncapp.State, profile, folder, bucket, prefix string) {
	st.Profile.Folder = folder
	st.Profile.Bucket = bucket
	st.Profile.Prefix = prefix
	_ = st.Save(profile)
}

func printResult(res syncapp.SyncResult) {
	fmt.Printf("sync: uploaded=%d downloaded=%d skipped=%d deleted_local=%d deleted_remote=%d conflicts=%d errors=%d\n",
		res.Uploaded, res.Downloaded, res.Skipped, res.DeletedLocal, res.DeletedRemote, len(res.Conflicts), len(res.Errors))
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "  error: %s\n", e)
	}
	for _, c := range res.Conflicts {
		fmt.Fprintf(os.Stderr, "  conflict: %s -> %s\n", c.RelativePath, c.BackupPath)
	}
}

func fail(err any) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
