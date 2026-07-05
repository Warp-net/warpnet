//go:build mobile

package node

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	logging "github.com/ipfs/go-log/v2"
)

// Log levels accepted by LogSink.Write and SetLogSinkMinLevel.
const (
	LogLevelDebug = 0
	LogLevelInfo  = 1
	LogLevelWarn  = 2
	LogLevelError = 3
)

// LogSink receives structured log lines from the embedded Go node. The
// Kotlin side implements it and registers it via SetLogSink; gomobile
// generates the JNI glue. Write is called from arbitrary goroutines.
type LogSink interface {
	Write(level int, component string, msg string)
}

var (
	logSinkMu       sync.RWMutex
	logSink         LogSink
	logSinkMinLevel atomic.Int32 // defaults to LogLevelInfo, see init
	logPipeOnce     sync.Once
)

func init() {
	logSinkMinLevel.Store(LogLevelInfo)
}

// SetLogSink registers sink as an additional destination for node logs.
// stdout/stderr output (adb logcat, GoLog tag) is untouched. Passing nil
// detaches the current sink. Safe to call from any thread; call it after
// Initialize and before Connect so startup logs are captured.
func SetLogSink(sink LogSink) {
	logSinkMu.Lock()
	logSink = sink
	logSinkMu.Unlock()
	if sink == nil {
		return
	}
	logPipeOnce.Do(startGoLogPipe)
	applyGoLogLevel()
	sinkWrite(LogLevelInfo, "node", "log sink attached")
}

// SetLogSinkMinLevel sets the minimum level (LogLevelDebug..LogLevelError)
// forwarded to the sink. Default is LogLevelInfo so libp2p debug chatter
// doesn't flood the JNI boundary. While a sink is attached it also aligns
// go-log's per-logger levels, so lowering the filter to debug actually
// makes libp2p emit debug lines.
func SetLogSinkMinLevel(level int) {
	if level < LogLevelDebug {
		level = LogLevelDebug
	}
	if level > LogLevelError {
		level = LogLevelError
	}
	logSinkMinLevel.Store(int32(level))

	logSinkMu.RLock()
	attached := logSink != nil
	logSinkMu.RUnlock()
	if attached {
		applyGoLogLevel()
	}
}

// sinkWrite mirrors one log line into the registered sink, if any. A
// panicking Java-side implementation must not crash the node, hence the
// recover.
func sinkWrite(level int, component, msg string) {
	if int32(level) < logSinkMinLevel.Load() {
		return
	}
	logSinkMu.RLock()
	sink := logSink
	logSinkMu.RUnlock()
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink.Write(level, component, msg)
}

// logf prints to stdout exactly like the fmt calls it replaces (gomobile
// forwards stdout to logcat under the GoLog tag) and additionally mirrors
// the line into the LogSink with component "node".
func logf(level int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	sinkWrite(level, "node", msg)
}

// startGoLogPipe tees everything go-log (libp2p and friends) writes into
// the sink. The pipe is synchronous: if the reader stalls, logging inside
// libp2p blocks, so the loop keeps draining even on malformed lines and
// only exits when the pipe itself is closed.
func startGoLogPipe() {
	reader := logging.NewPipeReader(logging.PipeFormat(logging.JSONOutput))
	go func() {
		br := bufio.NewReader(reader)
		for {
			line, err := br.ReadString('\n')
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				forwardGoLogLine(trimmed)
			}
			if err != nil {
				return
			}
		}
	}()
}

// forwardGoLogLine parses one JSON entry produced by go-log's zap encoder
// and mirrors it into the sink with component "libp2p".
func forwardGoLogLine(line string) {
	var entry struct {
		Level  string `json:"level"`
		Logger string `json:"logger"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		sinkWrite(LogLevelInfo, "libp2p", line)
		return
	}
	msg := entry.Msg
	if entry.Logger != "" {
		msg = entry.Logger + ": " + msg
	}
	sinkWrite(goLogSinkLevel(entry.Level), "libp2p", msg)
}

func goLogSinkLevel(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	default: // error, dpanic, panic, fatal
		return LogLevelError
	}
}

// applyGoLogLevel aligns go-log's logger levels with the sink filter.
func applyGoLogLevel() {
	logging.SetAllLoggers(goLogLevel(int(logSinkMinLevel.Load())))
}

func goLogLevel(level int) logging.LogLevel {
	switch level {
	case LogLevelDebug:
		return logging.LevelDebug
	case LogLevelInfo:
		return logging.LevelInfo
	case LogLevelWarn:
		return logging.LevelWarn
	default:
		return logging.LevelError
	}
}
