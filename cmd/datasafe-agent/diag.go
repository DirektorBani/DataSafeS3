package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

type logEntry struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

func (a *App) logDir() string {
	if a != nil && a.log != nil && a.log.dir != "" {
		return a.log.dir
	}
	if a == nil || a.dataDir == "" {
		return ""
	}
	return filepath.Join(a.dataDir, "logs")
}

func versionsText(server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		server = "none"
	}
	return fmt.Sprintf("product: DataSafeS3 Desktop\nversion: %s\ncommit: %s\nos: %s\nserver: %s\ngo: %s\n", DesktopVersion, desktopCommit(), osDescription(), server, runtime.Version())
}

// desktopCommit prefers the CI ldflag. A local build leaves that as
// "unknown"; Go then has the revision it read with git while compiling.
func desktopCommit() string {
	return chooseCommit(DesktopCommit, vcsRevision())
}

func chooseCommit(explicit, revision string) string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" && !strings.EqualFold(explicit, "unknown") {
		return explicit
	}
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return "unknown"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return revision
}

func vcsRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

func (a *App) connectedServer(params map[string]any) string {
	if a == nil || params == nil {
		return ""
	}
	id := strParam(params, "id")
	if id == "" {
		return ""
	}
	profile, err := a.profileByID(id)
	if err != nil || profile.Server == nil {
		return ""
	}
	return profile.Server.ServerVersion
}

func isLogName(name string) bool {
	base := filepath.Base(name)
	if base != name || base == "" || base == "." || base == ".." {
		return false
	}
	switch {
	case base == "agent.log" || strings.HasPrefix(base, "agent.log."):
		return true
	case base == "desktop.log" || strings.HasPrefix(base, "desktop.log."):
		return true
	default:
		return false
	}
}

func listLogFiles(dir string) ([]logEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []logEntry
	for _, ent := range entries {
		if ent.IsDir() || !isLogName(ent.Name()) {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, logEntry{Name: ent.Name(), Bytes: info.Size()})
	}
	return out, nil
}

// diagPreview lists the log files and the version text. The window shows
// this before it asks the agent to write the archive (S12).
func (a *App) diagPreview(params map[string]any) (any, error) {
	if a.log != nil {
		a.log.Sync()
	}
	files, err := listLogFiles(a.logDir())
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []logEntry{}
	}
	return map[string]any{
		"files":    files,
		"versions": versionsText(a.connectedServer(params)),
	}, nil
}

// diagExport writes a zip of the log files and versions.txt into a directory
// the user already chose. profiles.json and transfers.db are not included.
func (a *App) diagExport(params map[string]any) (any, error) {
	dest := strings.TrimSpace(strParam(params, "dest_path"))
	info, err := os.Stat(dest)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("diagnostics folder is required")
	}
	if a.log != nil {
		a.log.Sync()
	}
	files, err := listLogFiles(a.logDir())
	if err != nil {
		return nil, err
	}
	outPath := filepath.Join(dest, "datasafe-desktop-diagnostics.zip")
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	names := []string{"versions.txt"}
	vw, err := zw.Create("versions.txt")
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if _, err := io.WriteString(vw, versionsText(a.connectedServer(params))); err != nil {
		_ = zw.Close()
		return nil, err
	}
	for _, file := range files {
		if !isLogName(file.Name) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(a.logDir(), file.Name))
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		w, err := zw.Create(file.Name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := w.Write(body); err != nil {
			_ = zw.Close()
			return nil, err
		}
		names = append(names, file.Name)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return map[string]any{"path": outPath, "files": names}, nil
}
