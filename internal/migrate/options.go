package migrate

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Endpoint is one S3-compatible server.
type Endpoint struct {
	URL       string
	AccessKey string
	SecretKey string
	Region    string
}

// Options configures storage-cli migrate from-minio.
type Options struct {
	Source       Endpoint
	Targets      []Endpoint
	Buckets      []string
	StateDir     string
	SSE          string
	KMSKey       string
	Concurrency  int
	RateBytes    int64
	VerifySample float64
	VerifyFull   bool
	PartSize     int64
	ReportDir    string
	Stdout       io.Writer
	Stderr       io.Writer
	// AfterObject is called after each copied or skipped version. Tests use it to interrupt.
	AfterObject func(done int)
}

const (
	sourceVersionMeta = "x-amz-meta-datasafe-source-version"
	defaultPartSize   = 8 << 20
)

// ParseFromMinIO reads `migrate from-minio` arguments.
func ParseFromMinIO(args []string) (Options, error) {
	opt := Options{
		StateDir:    "./.minio-migrate-state",
		Concurrency: 4,
		PartSize:    defaultPartSize,
		ReportDir:   "./minio-migrate-report",
		Source:      Endpoint{Region: "us-east-1"},
	}
	var targets []string
	target := Endpoint{Region: "us-east-1"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		need := func() (string, error) {
			i++
			if i >= len(args) {
				return "", fmt.Errorf("%s needs a value", a)
			}
			return args[i], nil
		}
		switch a {
		case "--source":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Source.URL = v
		case "--source-key":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Source.AccessKey = v
		case "--source-secret":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Source.SecretKey = v
		case "--source-region":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.Source.Region = v
		case "--target":
			v, err := need()
			if err != nil {
				return opt, err
			}
			targets = append(targets, v)
		case "--target-key":
			v, err := need()
			if err != nil {
				return opt, err
			}
			target.AccessKey = v
		case "--target-secret":
			v, err := need()
			if err != nil {
				return opt, err
			}
			target.SecretKey = v
		case "--target-region":
			v, err := need()
			if err != nil {
				return opt, err
			}
			target.Region = v
		case "--buckets":
			v, err := need()
			if err != nil {
				return opt, err
			}
			for _, b := range strings.Split(v, ",") {
				b = strings.TrimSpace(b)
				if b != "" {
					opt.Buckets = append(opt.Buckets, b)
				}
			}
		case "--state":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.StateDir = v
		case "--sse":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.SSE = v
		case "--kms-key":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.KMSKey = v
		case "--concurrency":
			v, err := need()
			if err != nil {
				return opt, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return opt, fmt.Errorf("invalid --concurrency %q", v)
			}
			opt.Concurrency = n
		case "--rate":
			v, err := need()
			if err != nil {
				return opt, err
			}
			n, err := parseRate(v)
			if err != nil {
				return opt, err
			}
			opt.RateBytes = n
		case "--verify-sample":
			v, err := need()
			if err != nil {
				return opt, err
			}
			v = strings.TrimSuffix(v, "%")
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || n < 0 || n > 100 {
				return opt, fmt.Errorf("invalid --verify-sample %q", v)
			}
			opt.VerifySample = n
		case "--verify-full":
			opt.VerifyFull = true
		case "--part-size":
			v, err := need()
			if err != nil {
				return opt, err
			}
			n, err := parseRate(v)
			if err != nil || n < 1 {
				return opt, fmt.Errorf("invalid --part-size %q", v)
			}
			opt.PartSize = n
		case "--report":
			v, err := need()
			if err != nil {
				return opt, err
			}
			opt.ReportDir = v
		default:
			return opt, fmt.Errorf("unknown flag %q", a)
		}
	}
	if opt.Source.URL == "" {
		return opt, fmt.Errorf("--source is required")
	}
	if len(targets) == 0 {
		return opt, fmt.Errorf("--target is required")
	}
	if opt.Source.AccessKey == "" || opt.Source.SecretKey == "" {
		return opt, fmt.Errorf("--source-key and --source-secret are required")
	}
	if target.AccessKey == "" || target.SecretKey == "" {
		return opt, fmt.Errorf("--target-key and --target-secret are required")
	}
	if opt.SSE != "" && !strings.EqualFold(opt.SSE, "aws:kms") && !strings.EqualFold(opt.SSE, "AES256") {
		return opt, fmt.Errorf("--sse must be aws:kms or AES256")
	}
	if strings.EqualFold(opt.SSE, "aws:kms") && opt.KMSKey == "" {
		return opt, fmt.Errorf("--kms-key is required with --sse aws:kms")
	}
	for _, u := range targets {
		ep := target
		ep.URL = u
		opt.Targets = append(opt.Targets, ep)
	}
	return opt, nil
}

func parseRate(v string) (int64, error) {
	s := strings.TrimSpace(strings.ToUpper(v))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "MB"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		mult = 1 << 10
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", v)
	}
	return n * mult, nil
}

func (o Options) logf(format string, args ...any) {
	w := o.Stderr
	if w == nil {
		return
	}
	fmt.Fprintf(w, time.Now().Format("15:04:05 ")+format+"\n", args...)
}
