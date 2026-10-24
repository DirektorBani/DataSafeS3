package consolei18n

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestConsoleLocales is the CI gate (go test ./...) that the web console
// ships Russian and English with the same keys and no other locale.
func TestConsoleLocales(t *testing.T) {
	root := localesRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, entry := range entries {
		got[entry.Name()] = entry.IsDir()
		if entry.Name() != "ru" && entry.Name() != "en" {
			t.Errorf("unexpected locale entry %s", entry.Name())
		}
	}
	for _, lang := range []string{"ru", "en"} {
		if !got[lang] {
			t.Errorf("missing locale directory %s", lang)
		}
	}

	enFiles := jsonFiles(t, filepath.Join(root, "en"))
	ruFiles := jsonFiles(t, filepath.Join(root, "ru"))
	if strings.Join(enFiles, "\n") != strings.Join(ruFiles, "\n") {
		t.Fatalf("locale file lists differ\nen: %v\nru: %v", enFiles, ruFiles)
	}

	for _, name := range enFiles {
		enFlat := flattenLocale(t, filepath.Join(root, "en", name))
		ruFlat := flattenLocale(t, filepath.Join(root, "ru", name))
		enKeys := sortedKeys(enFlat)
		ruKeys := sortedKeys(ruFlat)
		if strings.Join(enKeys, "\n") != strings.Join(ruKeys, "\n") {
			t.Errorf("%s keys differ\nonly en: %v\nonly ru: %v", name, diffKeys(enFlat, ruFlat), diffKeys(ruFlat, enFlat))
		}
		for _, key := range enKeys {
			ru, ok := ruFlat[key]
			if !ok {
				continue
			}
			en := enFlat[key]
			if strings.TrimSpace(en) == "" {
				t.Errorf("en/%s %s is empty", name, key)
			}
			if strings.TrimSpace(ru) == "" {
				t.Errorf("ru/%s %s is empty", name, key)
			}
			if ph := placeholders(en); strings.Join(ph, "|") != strings.Join(placeholders(ru), "|") {
				t.Errorf("placeholder mismatch %s %s: en %q ru %q", name, key, ph, placeholders(ru))
			}
		}
	}
}

func localesRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "web", "console", "src", "locales")
}

func jsonFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			t.Errorf("%s: locale directories contain JSON files only", filepath.Join(dir, entry.Name()))
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func flattenLocale(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := map[string]string{}
	flatten(t, path, "", doc, out)
	return out
}

func flatten(t *testing.T, file, prefix string, doc map[string]any, out map[string]string) {
	t.Helper()
	for key, child := range doc {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		switch value := child.(type) {
		case map[string]any:
			flatten(t, file, full, value, out)
		case string:
			out[full] = value
		default:
			t.Errorf("%s %s: value must be a string", file, full)
		}
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func diffKeys(have, missing map[string]string) []string {
	var out []string
	for key := range have {
		if _, ok := missing[key]; !ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

var placeholderRE = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func placeholders(text string) []string {
	matches := placeholderRE.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, strings.TrimSpace(match[1]))
	}
	sort.Strings(out)
	return out
}
