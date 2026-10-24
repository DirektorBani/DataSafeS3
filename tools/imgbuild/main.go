// Command imgbuild assembles and pushes a container image WITHOUT a Docker
// daemon, without kaniko and without emulation: it takes a base image from a
// registry, appends layers made from files on disk, edits the image config and
// pushes a single image or a multi-platform index.
//
// It exists because the GitVerse cloud runner has no docker.sock, and the
// release must not depend on a build tool hosted outside the project's own
// registry. The DataSafeS3 images are "base image + static binaries", so no
// RUN step is needed (see deploy/docker/Dockerfile: the final stage is COPY only).
//
//	imgbuild --base dh-mirror.gitverse.ru/library/alpine:3.24 \
//	  --platform linux/amd64,linux/arm64 \
//	  --tag gitverse.ru/direktorbani/datasafe-storage-server:v1.7.1 \
//	  --file 'dist/linux_{arch}/storage-server=/storage-server:0755' \
//	  --entrypoint /usr/local/bin/datasafe-entrypoint --expose 9000/tcp
//
// Registry credentials come from the Docker config (DOCKER_CONFIG), as for any
// Docker client. Layer timestamps and the image creation time use
// SOURCE_DATE_EPOCH when set, so the same inputs give the same layers.
package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

type options struct {
	base        string
	platforms   string
	tags        multi
	files       multi
	dirs        multi
	env         multi
	labels      multi
	expose      multi
	entrypoint  string
	user        string
	healthcheck string
	hcInterval  time.Duration
	hcTimeout   time.Duration
	hcStart     time.Duration
	hcRetries   int
	epoch       time.Time
}

// layerSpec is one COPY-like step: a map of destination path -> source file.
type layerSpec struct {
	desc    string
	entries []entry
}

type entry struct {
	dst  string
	src  string // empty for directories
	mode int64
	dir  bool
}

func parseFile(spec, arch string) (src, dst string, mode int64, err error) {
	spec = strings.ReplaceAll(spec, "{arch}", arch)
	srcdst, modeStr, hasMode := cutLast(spec, ":")
	mode = 0o644
	if hasMode {
		if _, convErr := strconv.ParseInt(modeStr, 8, 32); convErr == nil && !strings.Contains(modeStr, "/") {
			mode, _ = strconv.ParseInt(modeStr, 8, 32)
		} else {
			srcdst = spec
		}
	}
	src, dst, ok := strings.Cut(srcdst, "=")
	if !ok || src == "" || !path.IsAbs(dst) {
		return "", "", 0, fmt.Errorf("--file %q: want SRC=/ABS/DST[:MODE]", spec)
	}
	return src, dst, mode, nil
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func buildSpecs(o *options, arch string) ([]layerSpec, error) {
	var specs []layerSpec
	for _, f := range o.files {
		src, dst, mode, err := parseFile(f, arch)
		if err != nil {
			return nil, err
		}
		if st, err := os.Stat(src); err != nil || st.IsDir() {
			return nil, fmt.Errorf("--file source %q is not a readable file", src)
		}
		specs = append(specs, layerSpec{
			desc:    fmt.Sprintf("COPY %s %s", filepath.Base(src), dst),
			entries: []entry{{dst: dst, src: src, mode: mode}},
		})
	}
	for _, d := range o.dirs {
		d = strings.ReplaceAll(d, "{arch}", arch)
		src, dst, ok := strings.Cut(d, "=")
		if !ok || src == "" || !path.IsAbs(dst) {
			return nil, fmt.Errorf("--dir %q: want SRC=/ABS/DST", d)
		}
		var es []entry
		err := filepath.WalkDir(src, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			target := path.Join(dst, filepath.ToSlash(rel))
			switch {
			case de.IsDir():
				es = append(es, entry{dst: target, mode: 0o755, dir: true})
			case de.Type().IsRegular():
				mode := int64(0o644)
				if info, e := de.Info(); e == nil && info.Mode()&0o111 != 0 {
					mode = 0o755
				}
				es = append(es, entry{dst: target, src: p, mode: mode})
			default:
				return fmt.Errorf("--dir %q: %s is not a regular file or directory", src, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Slice(es, func(i, j int) bool { return es[i].dst < es[j].dst })
		specs = append(specs, layerSpec{desc: fmt.Sprintf("COPY %s %s", filepath.Base(src), dst), entries: es})
	}
	return specs, nil
}

// makeTar writes a deterministic tar: parents first, sorted, root:root, fixed mtime.
func makeTar(es []entry, mtime time.Time) ([]byte, error) {
	seen := map[string]bool{}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	mkdirs := func(p string) error {
		var chain []string
		for d := path.Dir(p); d != "/" && d != "." && !seen[d]; d = path.Dir(d) {
			chain = append([]string{d}, chain...)
		}
		for _, d := range chain {
			seen[d] = true
			h := &tar.Header{Typeflag: tar.TypeDir, Name: strings.TrimPrefix(d, "/") + "/", Mode: 0o755, ModTime: mtime, Format: tar.FormatPAX}
			if err := tw.WriteHeader(h); err != nil {
				return err
			}
		}
		return nil
	}
	for _, e := range es {
		if e.dir {
			if err := mkdirs(path.Join(e.dst, "x")); err != nil {
				return nil, err
			}
			continue
		}
		if err := mkdirs(e.dst); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(e.src)
		if err != nil {
			return nil, err
		}
		h := &tar.Header{Typeflag: tar.TypeReg, Name: strings.TrimPrefix(e.dst, "/"), Mode: e.mode, Size: int64(len(data)), ModTime: mtime, Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildImage(o *options, p *v1.Platform) (v1.Image, error) {
	baseRef, err := name.ParseReference(o.base)
	if err != nil {
		return nil, err
	}
	img, err := remote.Image(baseRef, remote.WithPlatform(*p), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("pull base %s for %s: %w", o.base, p, err)
	}
	specs, err := buildSpecs(o, p.Architecture)
	if err != nil {
		return nil, err
	}
	for _, s := range specs {
		data, err := makeTar(s.entries, o.epoch)
		if err != nil {
			return nil, err
		}
		layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		}, tarball.WithCompressedCaching)
		if err != nil {
			return nil, err
		}
		img, err = mutate.Append(img, mutate.Addendum{
			Layer:   layer,
			History: v1.History{Created: v1.Time{Time: o.epoch}, CreatedBy: "imgbuild " + s.desc},
		})
		if err != nil {
			return nil, err
		}
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}
	cfg = cfg.DeepCopy()
	cfg.Created = v1.Time{Time: o.epoch}
	if o.entrypoint != "" {
		cfg.Config.Entrypoint = []string{o.entrypoint}
		cfg.Config.Cmd = nil // like Dockerfile ENTRYPOINT: the base image CMD is dropped
	}
	if o.user != "" {
		cfg.Config.User = o.user
	}
	cfg.Config.Env = append(cfg.Config.Env, o.env...)
	if len(o.labels) > 0 && cfg.Config.Labels == nil {
		cfg.Config.Labels = map[string]string{}
	}
	for _, l := range o.labels {
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			return nil, fmt.Errorf("--label %q: want KEY=VALUE", l)
		}
		cfg.Config.Labels[k] = v
	}
	if len(o.expose) > 0 && cfg.Config.ExposedPorts == nil {
		cfg.Config.ExposedPorts = map[string]struct{}{}
	}
	for _, e := range o.expose {
		cfg.Config.ExposedPorts[e] = struct{}{}
	}
	if o.healthcheck != "" {
		cfg.Config.Healthcheck = &v1.HealthConfig{
			Test:        []string{"CMD-SHELL", o.healthcheck},
			Interval:    o.hcInterval,
			Timeout:     o.hcTimeout,
			StartPeriod: o.hcStart,
			Retries:     o.hcRetries,
		}
	}
	return mutate.ConfigFile(img, cfg)
}

func run(o *options) error {
	if o.base == "" || len(o.tags) == 0 {
		return errors.New("--base and at least one --tag are required")
	}
	var plats []*v1.Platform
	for _, s := range strings.Split(o.platforms, ",") {
		p, err := v1.ParsePlatform(strings.TrimSpace(s))
		if err != nil {
			return err
		}
		plats = append(plats, p)
	}
	var idx v1.ImageIndex = empty.Index
	var single v1.Image
	for _, p := range plats {
		img, err := buildImage(o, p)
		if err != nil {
			return err
		}
		d, _ := img.Digest()
		fmt.Printf("built %s %s\n", p, d)
		single = img
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{Platform: p}})
	}
	for _, t := range o.tags {
		ref, err := name.ParseReference(t)
		if err != nil {
			return err
		}
		auth := remote.WithAuthFromKeychain(authn.DefaultKeychain)
		if len(plats) == 1 {
			err = remote.Write(ref, single, auth)
		} else {
			err = remote.WriteIndex(ref, idx, auth)
		}
		if err != nil {
			return fmt.Errorf("push %s: %w", t, err)
		}
		fmt.Printf("pushed %s\n", t)
	}
	return nil
}

func main() {
	o := &options{}
	flag.StringVar(&o.base, "base", "", "base image reference")
	flag.StringVar(&o.platforms, "platform", "linux/amd64", "comma separated platforms; {arch} in --file/--dir is replaced per platform")
	flag.Var(&o.tags, "tag", "destination reference (repeatable)")
	flag.Var(&o.files, "file", "SRC=/ABS/DST[:OCTAL_MODE] (repeatable, one layer each)")
	flag.Var(&o.dirs, "dir", "SRC_DIR=/ABS/DST_DIR (repeatable, one layer each)")
	flag.Var(&o.env, "env", "KEY=VALUE (repeatable)")
	flag.Var(&o.labels, "label", "KEY=VALUE (repeatable)")
	flag.Var(&o.expose, "expose", "port/proto, e.g. 9000/tcp (repeatable)")
	flag.StringVar(&o.entrypoint, "entrypoint", "", "image entrypoint (a single executable path); clears the base CMD")
	flag.StringVar(&o.user, "user", "", "image user")
	flag.StringVar(&o.healthcheck, "healthcheck", "", "shell command for HEALTHCHECK CMD-SHELL")
	flag.DurationVar(&o.hcInterval, "healthcheck-interval", 10*time.Second, "")
	flag.DurationVar(&o.hcTimeout, "healthcheck-timeout", 3*time.Second, "")
	flag.DurationVar(&o.hcStart, "healthcheck-start-period", 5*time.Second, "")
	flag.IntVar(&o.hcRetries, "healthcheck-retries", 3, "")
	flag.Parse()
	o.epoch = time.Now().UTC().Truncate(time.Second)
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, "SOURCE_DATE_EPOCH:", err)
			os.Exit(2)
		}
		o.epoch = time.Unix(sec, 0).UTC()
	}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "imgbuild:", err)
		os.Exit(1)
	}
}
