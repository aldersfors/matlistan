package web

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/jalet/matlistan/internal/weekplan"
)

// jobs runs at most one planning job per week in the background. Jobs outlive the request
// that started them; the planner records their outcome on the week itself.
type jobs struct {
	mu      sync.Mutex
	active  map[weekplan.Key]bool
	wg      sync.WaitGroup
	timeout time.Duration
	log     zerolog.Logger
}

func newJobs(timeout time.Duration, log zerolog.Logger) *jobs {
	return &jobs{active: map[weekplan.Key]bool{}, timeout: timeout, log: log}
}

func (j *jobs) start(k weekplan.Key, fn func(context.Context) error) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.active[k] {
		return false
	}
	j.active[k] = true
	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		defer func() {
			j.mu.Lock()
			delete(j.active, k)
			j.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), j.timeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			j.log.Warn().Err(err).Str("week", k.String()).Msg("planning job failed")
		}
	}()
	return true
}

func (j *jobs) running(k weekplan.Key) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.active[k]
}

func (j *jobs) wait() { j.wg.Wait() }
