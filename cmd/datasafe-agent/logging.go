package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	levelDebug = 0
	levelInfo  = 1
	levelWarn  = 2
	levelError = 3
)

// Logger writes agent.log and rotates it. Every line is masked before it
// is written, at every level (S4).
type Logger struct {
	mu       sync.Mutex
	dir      string
	path     string
	file     *os.File
	size     int64
	maxBytes int64
	maxFiles int
	minLevel int
}

func openLogger(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	l := &Logger{
		dir:      dir,
		path:     filepath.Join(dir, "agent.log"),
		maxBytes: 10 << 20,
		maxFiles: 5,
		minLevel: levelInfo,
	}
	if mb := strings.TrimSpace(os.Getenv("DATASAFE_DESKTOP_LOG_MAX_MB")); mb != "" {
		if n, err := strconv.Atoi(mb); err == nil && n > 0 {
			l.maxBytes = int64(n) << 20
		}
	}
	if n := strings.TrimSpace(os.Getenv("DATASAFE_DESKTOP_LOG_MAX_FILES")); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 1 {
			l.maxFiles = v
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DATASAFE_DESKTOP_LOG_LEVEL"))) {
	case "debug":
		l.minLevel = levelDebug
	case "warn", "warning":
		l.minLevel = levelWarn
	case "error":
		l.minLevel = levelError
	}
	if err := l.reopen(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logger) Debug(msg string) { l.write(levelDebug, "DEBUG", msg) }
func (l *Logger) Info(msg string)  { l.write(levelInfo, "INFO", msg) }
func (l *Logger) Warn(msg string)  { l.write(levelWarn, "WARN", msg) }
func (l *Logger) Error(msg string) { l.write(levelError, "ERROR", msg) }

func (l *Logger) write(level int, name, msg string) {
	if l == nil || level < l.minLevel {
		return
	}
	line := time.Now().UTC().Format(time.RFC3339) + " " + name + " " + maskText(msg) + "\n"
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	if l.size > 0 && l.size+int64(len(line)) > l.maxBytes {
		l.rotate()
	}
	n, err := l.file.WriteString(line)
	if err == nil {
		l.size += int64(n)
		// Sync each line. A crash then keeps the lines already written;
		// Close is not the only flush.
		_ = l.file.Sync()
	}
}

func (l *Logger) rotate() {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	oldest := fmt.Sprintf("%s.%d", l.path, l.maxFiles-1)
	_ = os.Remove(oldest)
	for i := l.maxFiles - 2; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", l.path, i), fmt.Sprintf("%s.%d", l.path, i+1))
	}
	_ = os.Rename(l.path, l.path+".1")
	_ = l.reopen()
}

func (l *Logger) reopen() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	l.file = f
	l.size = info.Size()
	return nil
}

func (l *Logger) Sync() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Sync()
	}
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

var (
	logURL   = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)
	logAuth  = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?\S+`)
	logField = regexp.MustCompile(`(?i)("?(?:x-datasafe-mfa-token|x-amz-signature|x-amz-credential|x-amz-security-token|password|secret|token|mfa_code|mfa_token|access_key_secret|exchange_code)"?\s*[:=]\s*)(?:"[^"]*"|[^\s,}&]+)`)
)

// maskText removes secrets from one log line. Presigned query strings are
// dropped whole. Header and field values become a fixed marker.
func maskText(s string) string {
	s = logURL.ReplaceAllStringFunc(s, func(raw string) string {
		q := strings.IndexByte(raw, '?')
		if q < 0 {
			return raw
		}
		query := strings.ToLower(raw[q:])
		if strings.Contains(query, "x-amz-") || strings.Contains(query, "password") || strings.Contains(query, "secret") || strings.Contains(query, "token") {
			return raw[:q] + "?[redacted]"
		}
		return raw
	})
	s = logAuth.ReplaceAllString(s, "${1}[redacted]")
	return logField.ReplaceAllString(s, "${1}[redacted]")
}
