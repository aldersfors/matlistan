package planner

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	_runs = promauto.NewCounterVec(prometheus.CounterOpts{Name: "matlistan_planner_runs_total",
		Help: "Planning runs by kind (week, swap) and outcome (ok, invalid, unavailable)."},
		[]string{"kind", "outcome"})
	_tokens = promauto.NewCounterVec(prometheus.CounterOpts{Name: "matlistan_planner_tokens_total",
		Help: "Model tokens by type (input, output, cache_read, cache_write)."}, []string{"type"})
	_duration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "matlistan_planner_duration_seconds", Help: "Wall time of one planning run.",
		Buckets: []float64{5, 15, 30, 60, 120, 240, 480}})
)

func countUsage(u Usage) {
	_tokens.WithLabelValues("input").Add(float64(u.Input))
	_tokens.WithLabelValues("output").Add(float64(u.Output))
	_tokens.WithLabelValues("cache_read").Add(float64(u.CacheRead))
	_tokens.WithLabelValues("cache_write").Add(float64(u.CacheWrite))
}
