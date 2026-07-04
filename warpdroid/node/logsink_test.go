//go:build mobile

package node

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

type recordedLine struct {
	level     int
	component string
	msg       string
}

type stubSink struct {
	mu    sync.Mutex
	lines []recordedLine
	ch    chan recordedLine
}

func newStubSink() *stubSink {
	return &stubSink{ch: make(chan recordedLine, 256)}
}

func (s *stubSink) Write(level int, component string, msg string) {
	line := recordedLine{level, component, msg}
	s.mu.Lock()
	s.lines = append(s.lines, line)
	s.mu.Unlock()
	select {
	case s.ch <- line:
	default:
	}
}

func (s *stubSink) snapshot() []recordedLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedLine(nil), s.lines...)
}

func (s *stubSink) contains(msg string) bool {
	for _, line := range s.snapshot() {
		if strings.Contains(line.msg, msg) {
			return true
		}
	}
	return false
}

func resetLogSink(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SetLogSink(nil)
		SetLogSinkMinLevel(LogLevelInfo)
	})
}

func TestSetLogSinkReceivesWrites(t *testing.T) {
	resetLogSink(t)
	sink := newStubSink()
	SetLogSink(sink)

	sinkWrite(LogLevelInfo, "node", "hello sink")

	if !sink.contains("hello sink") {
		t.Fatalf("expected sink to receive the line, got: %v", sink.snapshot())
	}
	if !sink.contains("log sink attached") {
		t.Fatalf("expected attach marker line, got: %v", sink.snapshot())
	}
}

func TestSinkWriteWithoutSinkIsNoop(t *testing.T) {
	resetLogSink(t)
	SetLogSink(nil)
	// Must not panic or block.
	sinkWrite(LogLevelError, "node", "dropped")
}

func TestLogSinkMinLevelFilters(t *testing.T) {
	resetLogSink(t)
	sink := newStubSink()
	SetLogSink(sink)
	SetLogSinkMinLevel(LogLevelWarn)

	sinkWrite(LogLevelDebug, "node", "level debug line")
	sinkWrite(LogLevelInfo, "node", "level info line")
	sinkWrite(LogLevelWarn, "node", "level warn line")
	sinkWrite(LogLevelError, "node", "level error line")

	if sink.contains("level debug line") || sink.contains("level info line") {
		t.Fatalf("lines below min level leaked through: %v", sink.snapshot())
	}
	if !sink.contains("level warn line") || !sink.contains("level error line") {
		t.Fatalf("lines at/above min level missing: %v", sink.snapshot())
	}
}

func TestSetLogSinkMinLevelClamps(t *testing.T) {
	resetLogSink(t)

	SetLogSinkMinLevel(-42)
	if got := logSinkMinLevel.Load(); got != LogLevelDebug {
		t.Fatalf("expected clamp to debug, got %d", got)
	}

	SetLogSinkMinLevel(42)
	if got := logSinkMinLevel.Load(); got != LogLevelError {
		t.Fatalf("expected clamp to error, got %d", got)
	}
}

type panicSink struct{}

func (panicSink) Write(int, string, string) { panic("sink exploded") }

func TestSinkPanicIsRecovered(t *testing.T) {
	resetLogSink(t)
	SetLogSink(panicSink{})
	// A throwing Java-side sink must not crash the node.
	sinkWrite(LogLevelError, "node", "boom")
	logf(LogLevelError, "boom via logf %d", 1)
}

func TestGoLogPipeForwardsToSink(t *testing.T) {
	resetLogSink(t)
	sink := newStubSink()
	SetLogSink(sink)

	logging.Logger("logsink-pipe-test").Error("pipe boom")

	deadline := time.After(5 * time.Second)
	for {
		select {
		case line := <-sink.ch:
			if strings.Contains(line.msg, "pipe boom") {
				if line.component != "libp2p" {
					t.Fatalf("expected component libp2p, got %q", line.component)
				}
				if line.level != LogLevelError {
					t.Fatalf("expected error level, got %d", line.level)
				}
				if !strings.Contains(line.msg, "logsink-pipe-test") {
					t.Fatalf("expected logger name in message, got %q", line.msg)
				}
				return
			}
		case <-deadline:
			t.Fatalf("go-log line never reached the sink: %v", sink.snapshot())
		}
	}
}

func TestGoLogSinkLevelMapping(t *testing.T) {
	cases := map[string]int{
		"debug":  LogLevelDebug,
		"info":   LogLevelInfo,
		"warn":   LogLevelWarn,
		"error":  LogLevelError,
		"dpanic": LogLevelError,
		"fatal":  LogLevelError,
		"":       LogLevelError,
	}
	for in, want := range cases {
		if got := goLogSinkLevel(in); got != want {
			t.Fatalf("goLogSinkLevel(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLogSinkConcurrency(t *testing.T) {
	resetLogSink(t)
	sink := newStubSink()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				sinkWrite(LogLevelError, "node", fmt.Sprintf("writer %d line %d", n, j))
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 50; j++ {
			SetLogSink(sink)
			SetLogSink(nil)
		}
		SetLogSink(sink)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			SetLogSinkMinLevel(j % 4)
		}
		SetLogSinkMinLevel(LogLevelDebug)
	}()
	wg.Wait()

	sinkWrite(LogLevelError, "node", "after concurrency")
	if !sink.contains("after concurrency") {
		t.Fatalf("sink not functional after concurrent access: %v", sink.snapshot())
	}
}
