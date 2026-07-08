package daemon

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeRunner is a threadsafe CycleRunner that counts how many cycles ran.
type fakeRunner struct {
	mu    sync.Mutex
	count int
}

func (r *fakeRunner) RunCycle(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	return nil
}

func (r *fakeRunner) getCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// fakeLoopConfig is a minimal Config implementation for driving the loop.
// Only GetInterval is meaningful; the rest return zero values.
type fakeLoopConfig struct{}

func (fakeLoopConfig) GetInterval() time.Duration { return 30 * time.Minute }
func (fakeLoopConfig) GetHTTPPort() int           { return 0 }
func (fakeLoopConfig) IsMetricsEnabled() bool     { return false }
func (fakeLoopConfig) IsDryRun() bool             { return false }
func (fakeLoopConfig) GetProjectID() string       { return "test" }

// TestAutoscalingLoopFakeClock exercises the daemon's autoscaling loop using
// the Go 1.25+ testing/synctest fake clock. Inside synctest.Test the clock is
// virtual: time.Sleep and the loop's time.NewTicker advance instantly, so this
// test simulates multiple 30-minute intervals in essentially zero real
// wall-clock time. synctest.Wait blocks until every goroutine in the bubble is
// durably blocked, letting us assert on cycle counts deterministically.
func TestAutoscalingLoopFakeClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &fakeRunner{}
		ctx, cancel := context.WithCancel(context.Background())
		d := &Daemon{
			config: fakeLoopConfig{},
			runner: r,
			ctx:    ctx,
			cancel: cancel,
		}

		d.wg.Add(1)
		go d.autoscalingLoop()

		// The immediate startup cycle runs, then the loop blocks in select.
		synctest.Wait()
		if got := r.getCount(); got != 1 {
			t.Fatalf("after startup: expected count 1, got %d", got)
		}

		// Advance the fake clock by one interval so the ticker fires.
		time.Sleep(30 * time.Minute)
		synctest.Wait()
		if got := r.getCount(); got != 2 {
			t.Fatalf("after 1 interval: expected count 2, got %d", got)
		}

		time.Sleep(30 * time.Minute)
		synctest.Wait()
		if got := r.getCount(); got != 3 {
			t.Errorf("after 2 intervals: expected count 3, got %d", got)
		}

		cancel()
		synctest.Wait()
		d.wg.Wait()
	})
}
