package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// OSName selects the path rules in spec 4.7.3. Tests pass Windows rules on
// Linux; the desktop build itself is Windows and Linux only.
type OSName string

const (
	OSWindows OSName = "windows"
	OSLinux   OSName = "linux"
	OSDarwin  OSName = "darwin"
)

func hostOS() OSName {
	switch runtime.GOOS {
	case "windows":
		return OSWindows
	case "darwin":
		return OSDarwin
	default:
		return OSLinux
	}
}

// PathNote is one rename or skip shown on the transfer.
type PathNote struct {
	Action string `json:"action"`
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason"`
}

// MapDownloadPath turns an object key into a path inside destDir for osName.
// An empty path means the key was skipped. used tracks names already chosen
// so a case-only collision on Windows or macOS gets a suffix.
func MapDownloadPath(key, destDir string, osName OSName, used map[string]struct{}, longPaths bool) (string, []PathNote) {
	if used == nil {
		used = map[string]struct{}{}
	}
	original := key
	rel, notes, skip := cleanKey(key, osName)
	if skip || rel == "" {
		return "", append(notes, PathNote{Action: "skip", From: original, Reason: "path"})
	}
	full := joinOS(destDir, rel, osName)
	if !insideDir(destDir, full, osName) {
		return "", []PathNote{{Action: "skip", From: original, Reason: "escape"}}
	}
	if tooLong(full, rel, osName, longPaths) {
		return "", []PathNote{{Action: "skip", From: original, Reason: "too_long"}}
	}
	folded := foldName(full, osName)
	if _, taken := used[folded]; taken {
		base := full
		for n := 2; n < 10000; n++ {
			full = suffixed(base, n, osName)
			folded = foldName(full, osName)
			if _, taken = used[folded]; !taken {
				break
			}
		}
		notes = append(notes, PathNote{Action: "rename", From: original, To: full, Reason: "case"})
	}
	used[folded] = struct{}{}
	for i := range notes {
		if notes[i].Action == "rename" && notes[i].To == "" {
			notes[i].To = full
		}
	}
	return full, notes
}

// UploadKey joins a prefix and a relative local path with '/'. Segments "."
// and ".." are dropped so the key stays under the prefix. Object keys are
// not renamed for the source OS; download applies those rules.
func UploadKey(prefix, rel string) (string, PathNote, bool) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	var parts []string
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		parts = append(parts, seg)
	}
	if len(parts) == 0 {
		return "", PathNote{Action: "skip", From: rel, Reason: "path"}, false
	}
	key := strings.Join(parts, "/")
	prefix = strings.Trim(strings.ReplaceAll(prefix, "\\", "/"), "/")
	if prefix != "" {
		key = prefix + "/" + key
	}
	return key, PathNote{}, true
}

func cleanKey(key string, osName OSName) (string, []PathNote, bool) {
	raw := strings.ReplaceAll(key, "\\", "/")
	raw = strings.TrimLeft(raw, "/")
	var notes []PathNote
	var parts []string
	renamed := false
	for _, seg := range strings.Split(raw, "/") {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		next, changed, skip, reason := sanitizeSegment(seg, osName)
		if skip {
			return "", []PathNote{{Action: "skip", From: key, Reason: reason}}, true
		}
		if changed {
			renamed = true
		}
		if next == "" {
			return "", []PathNote{{Action: "skip", From: key, Reason: "path"}}, true
		}
		parts = append(parts, next)
	}
	if len(parts) == 0 {
		return "", nil, true
	}
	rel := strings.Join(parts, string(sep(osName)))
	if renamed {
		notes = append(notes, PathNote{Action: "rename", From: key, Reason: "name"})
	}
	return rel, notes, false
}

func sanitizeSegment(seg string, osName OSName) (string, bool, bool, string) {
	var changed bool
	if osName == OSDarwin {
		next := norm.NFC.String(seg)
		if next != seg {
			changed = true
		}
		seg = next
	}
	if osName == OSLinux || osName == OSDarwin {
		if len(seg) > 255 {
			return "", false, true, "segment"
		}
	}
	var b strings.Builder
	for _, r := range seg {
		if r == 0 {
			if osName == OSLinux || osName == OSDarwin || osName == OSWindows {
				b.WriteByte('_')
				changed = true
				continue
			}
		}
		if osName == OSWindows && (r < 0x20 || strings.ContainsRune(`<>:"|?*`, r)) {
			b.WriteByte('_')
			changed = true
			continue
		}
		if osName == OSDarwin && (r == ':' || r == '/') {
			b.WriteByte('_')
			changed = true
			continue
		}
		if osName == OSLinux && r == '/' {
			b.WriteByte('_')
			changed = true
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if osName == OSWindows {
		i := len(out)
		for i > 0 && (out[i-1] == '.' || out[i-1] == ' ') {
			i--
		}
		if i != len(out) {
			out = out[:i] + strings.Repeat("_", len(out)-i)
			changed = true
		}
		if out == "" {
			out = "_"
			changed = true
		}
		if reservedWindows(out) {
			out += "_"
			changed = true
		}
	}
	if !utf8.ValidString(out) {
		return "", false, true, "name"
	}
	return out, changed, false, ""
}

func reservedWindows(name string) bool {
	base := name
	if cut := strings.IndexByte(name, '.'); cut >= 0 {
		base = name[:cut]
	}
	base = strings.ToUpper(base)
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		return base[3] >= '1' && base[3] <= '9'
	}
	return false
}

func sep(osName OSName) byte {
	if osName == OSWindows {
		return '\\'
	}
	return '/'
}

func joinOS(dir, rel string, osName OSName) string {
	dir = strings.TrimRight(dir, `/\`)
	if dir == "" {
		return rel
	}
	return dir + string(sep(osName)) + rel
}

func insideDir(dir, full string, osName OSName) bool {
	dir = strings.TrimRight(dir, `/\`)
	if dir == "" {
		return !strings.Contains(full, "..")
	}
	prefix := dir + string(sep(osName))
	if !strings.HasPrefix(strings.ToLower(full), strings.ToLower(prefix)) && foldName(full, osName) != foldName(dir, osName) {
		if !strings.HasPrefix(full, prefix) {
			return false
		}
	}
	rest := strings.TrimPrefix(full, dir)
	rest = strings.Trim(rest, `/\`)
	for _, seg := range strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return false
		}
	}
	return true
}

func tooLong(full, rel string, osName OSName, longPaths bool) bool {
	for _, seg := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if (osName == OSLinux || osName == OSDarwin) && len(seg) > 255 {
			return true
		}
	}
	switch osName {
	case OSWindows:
		return !longPaths && len(full) > 260
	case OSDarwin:
		return len(full) > 1024
	default:
		return len(full) > 4096
	}
}

func foldName(path string, osName OSName) string {
	if osName == OSWindows || osName == OSDarwin {
		return strings.ToLower(path)
	}
	return path
}

func suffixed(path string, n int, osName OSName) string {
	s := string(sep(osName))
	dir, name := "", path
	if cut := strings.LastIndex(path, s); cut >= 0 {
		dir, name = path[:cut], path[cut+1:]
	}
	ext := ""
	stem := name
	if cut := strings.LastIndex(name, "."); cut > 0 {
		stem, ext = name[:cut], name[cut:]
	}
	next := stem + "_" + itoa(n) + ext
	if dir == "" {
		return next
	}
	return dir + s + next
}

// extendPath adds the Windows \\?\ prefix so CreateFileW accepts a path
// longer than 260 characters. Other systems return the path unchanged.
func extendPath(path string) string {
	if runtime.GOOS != "windows" {
		return path
	}
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	if strings.HasPrefix(abs, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(abs, `\\`)
	}
	return `\\?\` + abs
}

func openLocal(path string) (*os.File, error) {
	return os.Open(extendPath(path))
}

func createLocal(path string) (*os.File, error) {
	if err := os.MkdirAll(extendPath(filepath.Dir(path)), 0o755); err != nil && runtime.GOOS == "windows" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(extendPath(path), os.O_CREATE|os.O_RDWR, 0o644)
}
