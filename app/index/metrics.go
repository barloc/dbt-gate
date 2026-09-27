package index

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricManifestGenerated = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dbt_gate_manifest_generated_unixtime",
		Help: "Unix time of dbt manifest metadata.generated_at in the loaded index",
	}, []string{"product"})

	metricIndexLoaded = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dbt_gate_index_loaded_unixtime",
		Help: "Unix time of last successful index refresh (load or 304 Not Modified)",
	}, []string{"product"})

	metricIndexReady = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dbt_gate_index_ready",
		Help: "1 if an index is loaded in memory, else 0",
	}, []string{"product"})

	metricRefreshErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dbt_gate_index_refresh_errors_total",
		Help: "Count of failed index refresh attempts (fetch or build)",
	}, []string{"product"})
)

func (s *Store) setReady(v float64) {
	metricIndexReady.WithLabelValues(s.cfg.Product).Set(v)
}

func (s *Store) observeLoaded() {
	metricIndexLoaded.WithLabelValues(s.cfg.Product).Set(float64(time.Now().Unix()))
}

func (s *Store) observeIndex(idx *Index) {
	if idx == nil {
		s.setReady(0)
		return
	}
	ts := parseGeneratedUnix(idx.Meta.ManifestGeneratedAt)
	switch {
	case idx.Meta.ManifestGeneratedAt == "":
		s.log.Warn("manifest generated_at missing; dbt_gate_manifest_generated_unixtime not updated")
	case ts <= 0:
		s.log.Warn("manifest generated_at unparseable; dbt_gate_manifest_generated_unixtime not updated",
			slog.String("generated_at", idx.Meta.ManifestGeneratedAt))
	default:
		metricManifestGenerated.WithLabelValues(s.cfg.Product).Set(ts)
	}
	s.observeLoaded()
	s.setReady(1)
}

func (s *Store) observeRefreshError() {
	metricRefreshErrors.WithLabelValues(s.cfg.Product).Inc()
}

func parseGeneratedUnix(s string) float64 {
	if s == "" {
		return 0
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return float64(t.Unix())
		}
	}
	return 0
}
