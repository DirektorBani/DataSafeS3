//go:build datasafe_sinkexec

package events

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/events/bridgeproto"
)

// KafkaBridgePath is the child process in the storage-server image.
// STORAGE_KAFKA_BRIDGE overrides it. The process is not started when brokers are empty.
const KafkaBridgePath = "/usr/lib/datasafe/datasafe-kafka-bridge"

const (
	bridgeRestartMin = 200 * time.Millisecond
	bridgeRestartMax = 30 * time.Second
	// Parent deadlines are longer than the child (probe 2s, produce 10s in
	// the bridge process). The child writes a reply within its own deadline
	// when the broker is unreachable. A parent deadline that fires while that
	// process is still running is not a restart.
	bridgeHealthIPC  = 5 * time.Second
	bridgePublishIPC = 15 * time.Second
	bridgeProbeEvery = 5 * time.Second
)

// bridgeLogger is the server log for child exits and child stderr.
// Tests replace it. The process is not started when brokers are empty, so
// these lines are absent on that path.
var (
	bridgeLogMu  sync.Mutex
	bridgeLogger = slog.Default()
)

func bridgeWarn(msg string, args ...any) {
	bridgeLogMu.Lock()
	l := bridgeLogger
	bridgeLogMu.Unlock()
	l.Warn(msg, args...)
}

func bridgeInfo(msg string, args ...any) {
	bridgeLogMu.Lock()
	l := bridgeLogger
	bridgeLogMu.Unlock()
	l.Info(msg, args...)
}

// execSink talks to the franz-go child over stdin and stdout. The client is
// not in this process. Health reads a cache. A background probe and Publish
// update it, so a handler never waits on the broker or on the child.
type execSink struct {
	path string

	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	waited   chan struct{}
	quiesced bool
	// late is a read left in flight when the caller's deadline fired and the
	// child process was still alive. The next call discards that frame
	// before writing another request. It is not a process failure.
	late chan bridgeRead

	statusMu      sync.Mutex
	status        string
	lastErr       string
	publishFailed bool

	backingOff bool
	backoff    time.Duration
	generation int
	timer      *time.Timer

	done sync.Once
	stop chan struct{}
}

// NewKafkaSink starts the child. Call it only when brokers are set.
// An empty broker list returns an error and does not start a process.
func NewKafkaSink(cfg KafkaConfig) (EventSink, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, fmt.Errorf("STORAGE_KAFKA_BROKERS is empty")
	}
	path := os.Getenv("STORAGE_KAFKA_BRIDGE")
	if path == "" {
		path = KafkaBridgePath
	}
	s := &execSink{path: path, status: "degraded", stop: make(chan struct{})}
	s.mu.Lock()
	err := s.start()
	s.mu.Unlock()
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	go s.probeLoop()
	return s, nil
}

func (s *execSink) Name() string { return "kafka" }

func (s *execSink) Publish(ctx context.Context, e Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.quiesced = false
	s.mu.Unlock()
	body, err := NotificationJSON(e)
	if err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, bridgePublishIPC)
		defer cancel()
	}
	key := KafkaRecordKey(e.TenantID, e.Bucket, e.Key)
	msg, err := s.roundTrip(ctx, bridgeproto.KindPub, []byte(key), body)
	if err != nil {
		s.notePublish(err)
		return err
	}
	if msg.Kind == bridgeproto.KindOK {
		s.notePublish(nil)
		return nil
	}
	text := "produce failed"
	if p, err := msg.Part(0); err == nil && len(p) > 0 {
		text = string(p)
	}
	pubErr := errors.New(text)
	s.notePublish(pubErr)
	return pubErr
}

// Health returns the cached status. It does not take the child lock and it
// does not ping.
func (s *execSink) Health(context.Context) (string, string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	st := s.status
	if st == "" {
		st = "degraded"
	}
	last := s.lastErr
	if st == "degraded" && last == "" {
		last = WaitingForConnection
	}
	return st, last
}

func (s *execSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quiesced = true
	s.generation++
	s.backingOff = false
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.done.Do(func() { close(s.stop) })
	return s.stopLocked()
}

func (s *execSink) probeLoop() {
	s.probeOnce()
	t := time.NewTicker(bridgeProbeEvery)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.probeOnce()
		}
	}
}

func (s *execSink) probeOnce() {
	select {
	case <-s.stop:
		return
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), bridgeHealthIPC)
	defer cancel()
	msg, err := s.roundTrip(ctx, bridgeproto.KindHealth)
	if err != nil {
		s.noteProbe("degraded", err.Error())
		return
	}
	if msg.Kind != bridgeproto.KindStatus {
		s.noteProbe("degraded", "unexpected health reply")
		return
	}
	status, err1 := msg.Part(0)
	last, err2 := msg.Part(1)
	if err1 != nil || err2 != nil || len(status) == 0 {
		s.noteProbe("degraded", "short health reply")
		return
	}
	s.noteProbe(string(status), string(last))
}

func (s *execSink) notePublish(err error) {
	s.statusMu.Lock()
	if err == nil {
		s.status = "connected"
		s.lastErr = ""
		s.publishFailed = false
		s.statusMu.Unlock()
		s.mu.Lock()
		s.backoff = bridgeRestartMin
		s.mu.Unlock()
		return
	}
	s.status = "degraded"
	s.lastErr = err.Error()
	s.publishFailed = true
	s.statusMu.Unlock()
}

func (s *execSink) noteProbe(status, last string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	if s.publishFailed {
		return
	}
	if status == "" {
		status = "degraded"
	}
	s.status = status
	s.lastErr = last
}

type bridgeRead struct {
	msg bridgeproto.Message
	err error
}

func (s *execSink) roundTrip(ctx context.Context, kind byte, parts ...[]byte) (bridgeproto.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return bridgeproto.Message{}, ctx.Err()
	}
	if s.quiesced {
		return bridgeproto.Message{}, errors.New("bridge stopped")
	}
	if s.backingOff {
		return bridgeproto.Message{}, errors.New(s.cachedErrLocked())
	}
	if err := s.drainLateLocked(ctx, kind); err != nil {
		return bridgeproto.Message{}, err
	}
	if s.cmd == nil {
		if err := s.start(); err != nil {
			s.noteChildDownLocked(err.Error())
			return bridgeproto.Message{}, err
		}
	}
	if err := bridgeproto.Write(s.stdin, kind, parts...); err != nil {
		return bridgeproto.Message{}, s.childFailedLocked("bridge exited: " + err.Error())
	}
	stdout := s.stdout
	ch := make(chan bridgeRead, 1)
	go func() {
		msg, err := bridgeproto.Read(stdout)
		ch <- bridgeRead{msg, err}
	}()
	select {
	case res := <-ch:
		return s.finishReadLocked(res)
	case <-ctx.Done():
		select {
		case res := <-ch:
			return s.finishReadLocked(res)
		default:
			if s.childAliveLocked() {
				s.late = ch
				return bridgeproto.Message{}, errors.New(timeoutReason(kind, ctx.Err()))
			}
			return bridgeproto.Message{}, s.childFailedLocked("bridge exited: " + ctx.Err().Error())
		}
	}
}

// drainLateLocked waits for a reply that belongs to a previous call. A frame
// from a child that is still running is discarded. A read error or a dead
// process is a real exit.
func (s *execSink) drainLateLocked(ctx context.Context, kind byte) error {
	ch := s.late
	if ch == nil {
		return nil
	}
	var waited <-chan struct{}
	if s.waited != nil {
		waited = s.waited
	}
	select {
	case res := <-ch:
		s.late = nil
		if res.err != nil {
			return s.childFailedLocked("bridge exited: " + res.err.Error())
		}
		return nil
	case <-waited:
		s.late = nil
		return s.childFailedLocked("bridge exited")
	case <-ctx.Done():
		return errors.New(timeoutReason(kind, ctx.Err()))
	}
}

func (s *execSink) finishReadLocked(res bridgeRead) (bridgeproto.Message, error) {
	if res.err != nil {
		return bridgeproto.Message{}, s.childFailedLocked("bridge exited: " + res.err.Error())
	}
	return res.msg, nil
}

func (s *execSink) childAliveLocked() bool {
	if s.cmd == nil || s.cmd.Process == nil || s.waited == nil {
		return false
	}
	select {
	case <-s.waited:
		return false
	default:
		return true
	}
}

func (s *execSink) childFailedLocked(reason string) error {
	_ = s.stopLocked()
	s.noteChildDownLocked(reason)
	return errors.New(reason)
}

// timeoutReason is a broker or deadline failure. It is not a process exit,
// and the text must not name the child binary (that path matches the
// single-node log check when brokers are empty; this path only runs when
// they are set).
func timeoutReason(kind byte, err error) string {
	what := "call"
	switch kind {
	case bridgeproto.KindPub:
		what = "produce"
	case bridgeproto.KindHealth:
		what = "probe"
	}
	if err == nil {
		return what + " timed out"
	}
	return what + " timed out: " + err.Error()
}

func (s *execSink) cachedErrLocked() string {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	if s.lastErr != "" {
		return s.lastErr
	}
	return "bridge down"
}

// noteChildDownLocked marks the sink degraded and restarts the child after
// backoff. The caller holds s.mu. Rows are not acked here: Publish returns
// the error and the dispatcher leaves them pending.
func (s *execSink) noteChildDownLocked(reason string) {
	s.generation++
	gen := s.generation
	s.backingOff = true
	s.statusMu.Lock()
	s.status = "degraded"
	if reason != "" {
		s.lastErr = reason
	}
	s.statusMu.Unlock()
	delay := s.takeBackoff()
	noteBridgeRestart()
	bridgeWarn("event bridge exited", "restart_in", delay.String(), "err", reason)
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(delay, func() { s.restart(gen) })
}

func (s *execSink) takeBackoff() time.Duration {
	d := s.backoff
	if d < bridgeRestartMin {
		d = bridgeRestartMin
	}
	next := d * 2
	if next > bridgeRestartMax {
		next = bridgeRestartMax
	}
	s.backoff = next
	return d
}

func (s *execSink) restart(gen int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.generation || s.quiesced {
		return
	}
	s.backingOff = false
	if err := s.start(); err != nil {
		s.noteChildDownLocked(err.Error())
	}
}

// start is called with s.mu held. The child gets the server environment and
// is killed if this process dies (see dieWithParent). It listens on no TCP port.
func (s *execSink) start() error {
	if _, err := os.Stat(s.path); err != nil {
		return fmt.Errorf("STORAGE_KAFKA_BROKERS is set but the client program at %s is not available: %w", s.path, err)
	}
	cmd := exec.Command(s.path)
	cmd.Env = os.Environ()
	dieWithParent(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReader(stdout)
	waited := make(chan struct{})
	s.waited = waited
	errBuf := &limitedBuf{max: 2048}
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()
			_, _ = errBuf.Write([]byte(line + "\n"))
			if line != "" {
				bridgeInfo("event bridge stderr", "line", line)
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		close(waited)
	}()
	type result struct {
		msg bridgeproto.Message
		err error
	}
	rd := s.stdout
	ch := make(chan result, 1)
	go func() {
		msg, err := bridgeproto.Read(rd)
		ch <- result{msg, err}
	}()
	select {
	case <-time.After(15 * time.Second):
		_ = s.stopLocked()
		return fmt.Errorf("client program at %s did not become ready", s.path)
	case <-waited:
		_ = s.stopLocked()
		extra := strings.TrimSpace(errBuf.String())
		if extra != "" {
			return fmt.Errorf("client program at %s exited during startup: %s", s.path, extra)
		}
		return fmt.Errorf("client program at %s exited during startup", s.path)
	case res := <-ch:
		if res.err != nil {
			_ = s.stopLocked()
			return res.err
		}
		if res.msg.Kind == bridgeproto.KindFail {
			text := "startup failed"
			if p, err := res.msg.Part(0); err == nil && len(p) > 0 {
				text = string(p)
			}
			_ = s.stopLocked()
			return errors.New(text)
		}
		if res.msg.Kind != bridgeproto.KindReady {
			_ = s.stopLocked()
			return fmt.Errorf("client program sent frame %d during startup", res.msg.Kind)
		}
		go s.watch(cmd, waited)
		return nil
	}
}

func (s *execSink) watch(cmd *exec.Cmd, waited <-chan struct{}) {
	<-waited
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != cmd {
		return
	}
	s.cmd = nil
	s.stdin = nil
	s.stdout = nil
	s.waited = nil
	s.late = nil
	if s.quiesced {
		return
	}
	s.noteChildDownLocked("bridge exited")
}

// limitedBuf keeps the start of a child process's stderr for a startup error.
type limitedBuf struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (l *limitedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.b) < l.max {
		n := l.max - len(l.b)
		if n > len(p) {
			n = len(p)
		}
		l.b = append(l.b, p[:n]...)
	}
	return len(p), nil
}

func (l *limitedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.b)
}

func (s *execSink) stopLocked() error {
	s.late = nil
	if s.cmd == nil {
		return nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	done := s.waited
	cmd := s.cmd
	s.cmd = nil
	s.stdin = nil
	s.stdout = nil
	s.waited = nil
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil
	}
}
