// Command su-exec runs a program as another user, replacing itself (exec).
//
// It replaces the Alpine `su-exec` package in the DataSafeS3 images, so the
// final image stage needs no `apk add` and can be assembled without running a
// foreign-architecture process (cross-platform builds without QEMU).
//
//	su-exec UID[:GID] COMMAND [ARGS...]
//
// Only numeric IDs are accepted: the images have no /etc/passwd entry for the
// unprivileged user.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func parseIDs(spec string) (uid, gid int, err error) {
	u, g, hasGroup := strings.Cut(spec, ":")
	uid, err = strconv.Atoi(u)
	if err != nil || uid < 0 {
		return 0, 0, fmt.Errorf("invalid uid %q: numeric id required", u)
	}
	gid = uid
	if hasGroup {
		gid, err = strconv.Atoi(g)
		if err != nil || gid < 0 {
			return 0, 0, fmt.Errorf("invalid gid %q: numeric id required", g)
		}
	}
	return uid, gid, nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: su-exec UID[:GID] COMMAND [ARGS...]")
		os.Exit(1)
	}
	uid, gid, err := parseIDs(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "su-exec:", err)
		os.Exit(1)
	}
	path, err := exec.LookPath(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "su-exec:", err)
		os.Exit(127)
	}
	// Order matters: supplementary groups and gid first, uid last.
	if err := syscall.Setgroups([]int{gid}); err != nil {
		fmt.Fprintln(os.Stderr, "su-exec: setgroups:", err)
		os.Exit(1)
	}
	if err := syscall.Setgid(gid); err != nil {
		fmt.Fprintln(os.Stderr, "su-exec: setgid:", err)
		os.Exit(1)
	}
	if err := syscall.Setuid(uid); err != nil {
		fmt.Fprintln(os.Stderr, "su-exec: setuid:", err)
		os.Exit(1)
	}
	if err := syscall.Exec(path, os.Args[2:], os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "su-exec: exec:", err)
		os.Exit(126)
	}
}
