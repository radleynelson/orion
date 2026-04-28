// Package applog provides a process-wide file logger at ~/.orion/orion.log
// that is shared by the Go backend and the frontend (via App.LogClient). It
// implements the Wails logger.Logger interface so wails.Run can route its own
// output here as well.
package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	maxSize    = 5 * 1024 * 1024 // rotate when file exceeds 5 MB
	timeFormat = "2006-01-02 15:04:05.000"
)

var (
	mu      sync.Mutex
	out     io.Writer = os.Stderr // fallback before Init
	logPath string
)

// Init opens (or rotates) the log file and starts duplicating output to both
// the file and stderr. Safe to call once at startup.
func Init() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".orion")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "orion.log")

	if info, err := os.Stat(path); err == nil && info.Size() > maxSize {
		_ = os.Rename(path, path+".1")
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}

	mu.Lock()
	logPath = path
	out = io.MultiWriter(f, os.Stderr)
	writeLine("INFO ", fmt.Sprintf("--- orion started pid=%d ---", os.Getpid()))
	mu.Unlock()

	raiseFDLimit()
}

// raiseFDLimit lifts the process's RLIMIT_NOFILE soft limit toward the hard
// cap. macOS GUI apps inherit a soft limit of 256, which is too low for an
// app that watches file trees, manages PTYs, and shells out to git/tmux —
// hitting it surfaces as "too many open files" in unrelated subsystems.
func raiseFDLimit() {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		Warnf("getrlimit failed: %v", err)
		return
	}
	soft := lim.Cur
	target := lim.Max
	// macOS hard cap can be huge; cap our request at 65536 which is plenty.
	const wanted = 65536
	if target > wanted {
		target = wanted
	}
	if soft >= target {
		Infof("fd limit already adequate: soft=%d hard=%d", lim.Cur, lim.Max)
		return
	}
	lim.Cur = target
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		// Try a smaller bump as a fallback.
		lim.Cur = 4096
		if err2 := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err2 != nil {
			Warnf("setrlimit failed (target=%d, fallback=4096): %v / %v", target, err, err2)
			return
		}
	}
	Infof("raised fd soft limit: %d -> %d (hard=%d)", soft, lim.Cur, lim.Max)
}

// Path returns the absolute path of the current log file (empty before Init).
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

func writeLine(level, msg string) {
	fmt.Fprintf(out, "%s %s %s\n", time.Now().Format(timeFormat), level, msg)
}

func write(level, msg string) {
	mu.Lock()
	defer mu.Unlock()
	writeLine(level, msg)
}

// Errorf logs an error-level message.
func Errorf(format string, args ...any) { write("ERROR", fmt.Sprintf(format, args...)) }

// Infof logs an info-level message.
func Infof(format string, args ...any) { write("INFO ", fmt.Sprintf(format, args...)) }

// Warnf logs a warning-level message.
func Warnf(format string, args ...any) { write("WARN ", fmt.Sprintf(format, args...)) }

// Debugf logs a debug-level message.
func Debugf(format string, args ...any) { write("DEBUG", fmt.Sprintf(format, args...)) }

// FromClient logs a frontend-originated message tagged with its source level.
func FromClient(level, msg string) {
	switch level {
	case "error":
		Errorf("[client] %s", msg)
	case "warn":
		Warnf("[client] %s", msg)
	case "info":
		Infof("[client] %s", msg)
	default:
		Debugf("[client] %s", msg)
	}
}

// WailsLogger adapts applog to the Wails logger.Logger interface.
type WailsLogger struct{}

func (WailsLogger) Print(message string)   { write("PRINT", message) }
func (WailsLogger) Trace(message string)   { write("TRACE", message) }
func (WailsLogger) Debug(message string)   { write("DEBUG", message) }
func (WailsLogger) Info(message string)    { write("INFO ", message) }
func (WailsLogger) Warning(message string) { write("WARN ", message) }
func (WailsLogger) Error(message string)   { write("ERROR", message) }
func (WailsLogger) Fatal(message string) {
	write("FATAL", message)
	log.Fatal(message)
}
