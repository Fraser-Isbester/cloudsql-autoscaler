package daemon

import (
	"errors"
	"fmt"
)

// Define sentinel errors for better error handling following Go best practices
var (
	ErrDaemonNotStarted = errors.New("daemon not started")
	ErrDaemonStopped    = errors.New("daemon stopped")
	ErrInvalidConfig    = errors.New("invalid daemon configuration")
	ErrAnalyzerFailed   = errors.New("analyzer operation failed")
	ErrHTTPServerFailed = errors.New("HTTP server failed")
)

// DaemonError wraps errors with context following Russ Cox's error handling principles
type DaemonError struct {
	Op    string // Operation that failed
	Err   error  // Underlying error
	Phase string // Phase of daemon operation (startup, running, shutdown)
}

func (e *DaemonError) Error() string {
	if e.Phase != "" {
		return fmt.Sprintf("daemon %s during %s: %s", e.Op, e.Phase, e.Err)
	}
	return fmt.Sprintf("daemon %s: %s", e.Op, e.Err)
}

func (e *DaemonError) Unwrap() error {
	return e.Err
}

// Is implements the errors.Is interface
func (e *DaemonError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// NewDaemonError creates a new DaemonError with context
func NewDaemonError(op, phase string, err error) *DaemonError {
	return &DaemonError{
		Op:    op,
		Phase: phase,
		Err:   err,
	}
}

// WrapError wraps an error with daemon context
func WrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	return &DaemonError{Op: op, Err: err}
}

// IsRecoverable determines if an error is recoverable and the daemon should continue
func IsRecoverable(err error) bool {
	var daemonErr *DaemonError
	if errors.As(err, &daemonErr) {
		// Analyzer failures are generally recoverable - we can retry next cycle
		return errors.Is(daemonErr.Err, ErrAnalyzerFailed)
	}
	return false
}
