package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestUnitIndexFreshnessMetrics(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "manifest.mini.json"))
	require.NoError(t, err)

	const product = "example_metrics_test"
	store := NewTestStore(product)
	require.Equal(t, 0.0, testutil.ToFloat64(metricIndexReady.WithLabelValues(product)))

	require.NoError(t, store.LoadBytes(body, "sha1mini"))
	require.Equal(t, 1.0, testutil.ToFloat64(metricIndexReady.WithLabelValues(product)))

	wantGen, err := time.Parse(time.RFC3339Nano, "2026-08-21T12:00:00.000000Z")
	require.NoError(t, err)
	require.Equal(t, float64(wantGen.Unix()), testutil.ToFloat64(metricManifestGenerated.WithLabelValues(product)))

	loaded := testutil.ToFloat64(metricIndexLoaded.WithLabelValues(product))
	require.InDelta(t, float64(time.Now().Unix()), loaded, 5)
}
