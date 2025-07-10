package daemon

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// osSignalHandler implements SignalHandler interface
// Single responsibility: handle OS signals for graceful shutdown
type osSignalHandler struct {
	shutdownCh chan struct{}
	done       chan struct{}
}

// NewOSSignalHandler creates a new OS signal handler
func NewOSSignalHandler() SignalHandler {
	return &osSignalHandler{
		shutdownCh: make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// WaitForShutdown returns a channel that will be closed when shutdown is requested
func (h *osSignalHandler) WaitForShutdown() <-chan struct{} {
	go h.handleSignals()
	return h.shutdownCh
}

// handleSignals listens for shutdown signals and triggers graceful shutdown
func (h *osSignalHandler) handleSignals() {
	defer close(h.done)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Printf("Received signal: %v, initiating graceful shutdown", sig)
	close(h.shutdownCh)
}

// testSignalHandler provides a controllable signal handler for testing
type testSignalHandler struct {
	shutdownCh chan struct{}
}

// NewTestSignalHandler creates a signal handler that can be manually triggered
func NewTestSignalHandler() *testSignalHandler {
	return &testSignalHandler{
		shutdownCh: make(chan struct{}),
	}
}

// WaitForShutdown returns the shutdown channel
func (h *testSignalHandler) WaitForShutdown() <-chan struct{} {
	return h.shutdownCh
}

// TriggerShutdown manually triggers shutdown (for testing)
func (h *testSignalHandler) TriggerShutdown() {
	close(h.shutdownCh)
}
