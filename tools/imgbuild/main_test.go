package main

import (
	"archive/tar"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestBuildMultiPlatformIndex(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")

	// A two-platform base image with a CMD that must be dropped.
	var idx v1.ImageIndex = empty.Index
	for _, arch := range []string{"amd64", "arm64"} {
		img, err := random.Image(64, 1)
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := img.ConfigFile()
		cfg = cfg.DeepCopy()
		cfg.OS, cfg.Architecture = "linux", arch
		cfg.Config.Cmd = []string{"/bin/sh"}
		img, err = mutate.ConfigFile(img, cfg)
		if err != nil {
			t.Fatal(err)
		}
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: arch}}})
	}
	baseRef, _ := name.ParseReference(host + "/base/alpine:3.24")
	if err := remote.WriteIndex(baseRef, idx); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for _, arch := range []string{"amd64", "arm64"} {
		if err := os.MkdirAll(filepath.Join(dir, arch), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, arch, "server"), []byte("bin-"+arch), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	site := filepath.Join(dir, "site")
	_ = os.MkdirAll(filepath.Join(site, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(site, "index.html"), []byte("<html/>"), 0o644)
	_ = os.WriteFile(filepath.Join(site, "assets", "a.js"), []byte("x"), 0o644)

	o := &options{
		base:        host + "/base/alpine:3.24",
		platforms:   "linux/amd64,linux/arm64",
		tags:        multi{host + "/ds/server:v1", host + "/ds/server:latest"},
		files:       multi{filepath.Join(dir, "{arch}", "server") + "=/opt/ds/server:0755"},
		dirs:        multi{site + "=/srv/console"},
		env:         multi{"A=b"},
		labels:      multi{"org.opencontainers.image.version=1"},
		expose:      multi{"9000/tcp"},
		entrypoint:  "/opt/ds/server",
		user:        "",
		healthcheck: "true",
		hcInterval:  10 * time.Second, hcTimeout: 3 * time.Second, hcStart: 5 * time.Second, hcRetries: 3,
		epoch: time.Unix(1700000000, 0).UTC(),
	}
	if err := run(o); err != nil {
		t.Fatal(err)
	}

	ref, _ := name.ParseReference(host + "/ds/server:latest")
	for _, arch := range []string{"amd64", "arm64"} {
		img, err := remote.Image(ref, remote.WithPlatform(v1.Platform{OS: "linux", Architecture: arch}))
		if err != nil {
			t.Fatal(err)
		}
		cfg, _ := img.ConfigFile()
		if cfg.Architecture != arch {
			t.Fatalf("arch %s: config says %s", arch, cfg.Architecture)
		}
		if len(cfg.Config.Entrypoint) != 1 || cfg.Config.Entrypoint[0] != "/opt/ds/server" || cfg.Config.Cmd != nil {
			t.Fatalf("entrypoint/cmd wrong: %v %v", cfg.Config.Entrypoint, cfg.Config.Cmd)
		}
		if cfg.Config.Healthcheck == nil || cfg.Config.Healthcheck.Test[0] != "CMD-SHELL" {
			t.Fatalf("healthcheck missing")
		}
		if _, ok := cfg.Config.ExposedPorts["9000/tcp"]; !ok {
			t.Fatalf("port not exposed")
		}
		files := map[string]string{}
		layers, _ := img.Layers()
		for _, l := range layers {
			rc, err := l.Uncompressed()
			if err != nil {
				t.Fatal(err)
			}
			tr := tar.NewReader(rc)
			for {
				h, err := tr.Next()
				if err != nil {
					break
				}
				if h.Typeflag == tar.TypeReg {
					b, _ := io.ReadAll(tr)
					if h.Uid != 0 || h.Gid != 0 {
						t.Fatalf("%s not owned by root", h.Name)
					}
					files[h.Name] = string(b)
				}
			}
			rc.Close()
		}
		if files["opt/ds/server"] != "bin-"+arch {
			t.Fatalf("%s: server file = %q", arch, files["opt/ds/server"])
		}
		if files["srv/console/index.html"] != "<html/>" || files["srv/console/assets/a.js"] != "x" {
			t.Fatalf("console files missing: %v", files)
		}
	}
	// both tags resolve to the same index
	a, _ := remote.Head(ref)
	ref2, _ := name.ParseReference(host + "/ds/server:v1")
	b, _ := remote.Head(ref2)
	if a.Digest != b.Digest {
		t.Fatalf("tags differ: %s vs %s", a.Digest, b.Digest)
	}
}

func TestParseFile(t *testing.T) {
	src, dst, mode, err := parseFile("a/{arch}/x=/usr/bin/x:0755", "arm64")
	if err != nil || src != "a/arm64/x" || dst != "/usr/bin/x" || mode != 0o755 {
		t.Fatalf("got %q %q %o %v", src, dst, mode, err)
	}
	if _, _, m, err := parseFile("a=/b", "amd64"); err != nil || m != 0o644 {
		t.Fatalf("default mode: %o %v", m, err)
	}
	if _, _, _, err := parseFile("a=b", "amd64"); err == nil {
		t.Fatal("relative destination must fail")
	}
}
