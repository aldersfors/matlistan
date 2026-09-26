package web

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/jalet/matlistan/internal/weekplan"
)

func TestJobsAreSingleFlightPerWeek(t *testing.T) {
	j := newJobs(time.Second, zerolog.Nop())
	release := make(chan struct{})
	var runs atomic.Int32
	fn := func(context.Context) error { runs.Add(1); <-release; return nil }
	k := weekplan.Key{Year: 2026, Week: 40}
	if !j.start(k, fn) || j.start(k, fn) {
		t.Fatal("second start for the same week should be refused")
	}
	if !j.running(k) || !j.start(weekplan.Key{Year: 2026, Week: 41}, func(context.Context) error {
		return nil
	}) {
		t.Fatal("another week must be able to run")
	}
	close(release)
	j.wait()
	if j.running(k) || runs.Load() != 1 {
		t.Fatalf("running %v, runs %d", j.running(k), runs.Load())
	}
}

func TestJobsHaveATimeout(t *testing.T) {
	j := newJobs(10*time.Millisecond, zerolog.Nop())
	done := make(chan error, 1)
	j.start(weekplan.Key{Year: 2026, Week: 40}, func(ctx context.Context) error {
		<-ctx.Done()
		done <- ctx.Err()
		return ctx.Err()
	})
	j.wait()
	if err := <-done; err == nil {
		t.Fatal("job context never ended")
	}
}
