package daemon

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type erroringRunner struct{ fakeRunner }

func (r *erroringRunner) RunCycle(ctx context.Context) error {
	_ = r.fakeRunner.RunCycle(ctx)
	return errors.New("cycle failed")
}

// fakeHTTP blocks in Start until Shutdown, like http.Server.ListenAndServe.
type fakeHTTP struct {
	mu       sync.Mutex
	started  bool
	shutdown bool
	stop     chan struct{}
}

func (h *fakeHTTP) Start() error {
	h.mu.Lock()
	h.started = true
	h.mu.Unlock()
	<-h.stop
	return nil
}

func (h *fakeHTTP) Shutdown(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shutdown = true
	close(h.stop)
	return nil
}

func (h *fakeHTTP) state() (bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.started, h.shutdown
}

type portConfig struct{ fakeLoopConfig }

func (portConfig) GetHTTPPort() int { return 8080 }

// TestDaemonLifecycle drives Start end to end on a fake clock: the first cycle
// runs immediately, failing cycles do not stop the loop, and a shutdown signal
// stops the loop and the HTTP server before Start returns.
func TestDaemonLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &erroringRunner{}
		h := &fakeHTTP{stop: make(chan struct{})}
		sig := NewTestSignalHandler()
		ctx, cancel := context.WithCancel(context.Background())
		d := &Daemon{config: portConfig{}, runner: r, httpServer: h, signalHandler: sig, ctx: ctx, cancel: cancel}

		returned := make(chan error, 1)
		go func() { returned <- d.Start() }()

		synctest.Wait()
		if got := r.getCount(); got != 1 {
			t.Fatalf("startup cycle: want 1, got %d", got)
		}
		if started, _ := h.state(); !started {
			t.Fatal("HTTP server was not started")
		}

		time.Sleep(2 * 30 * time.Minute)
		synctest.Wait()
		if got := r.getCount(); got != 3 {
			t.Fatalf("failing cycles must not stop the loop: want 3, got %d", got)
		}

		sig.TriggerShutdown()
		synctest.Wait()
		select {
		case err := <-returned:
			if err != nil {
				t.Fatalf("Start returned %v", err)
			}
		default:
			t.Fatal("Start did not return after shutdown signal")
		}
		if _, shut := h.state(); !shut {
			t.Fatal("HTTP server was not shut down")
		}

		time.Sleep(time.Hour)
		synctest.Wait()
		if got := r.getCount(); got != 3 {
			t.Fatalf("cycles ran after shutdown: got %d", got)
		}
	})
}
