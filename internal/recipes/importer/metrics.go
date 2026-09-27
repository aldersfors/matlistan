package importer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/aldersfors/matlistan/internal/planner"
)

var _tokens = promauto.NewCounterVec(prometheus.CounterOpts{Name: "matlistan_import_tokens_total",
	Help: "Model tokens used by recipe imports, by type."}, []string{"type"})

func countUsage(u planner.Usage) {
	_tokens.WithLabelValues("input").Add(float64(u.Input))
	_tokens.WithLabelValues("output").Add(float64(u.Output))
	_tokens.WithLabelValues("cache_read").Add(float64(u.CacheRead))
	_tokens.WithLabelValues("cache_write").Add(float64(u.CacheWrite))
}
