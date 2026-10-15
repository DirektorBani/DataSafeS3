package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestLegacyClassifyMetricsRegisteredAtZero(t *testing.T) {
	ResetLegacyClassifyMetricsForTest()
	RegisterLegacyClassifyMetrics()
	for _, result := range []string{"plaintext", "format0", "skipped", "error"} {
		got, ok := legacyClassifyValue(t, "datasafe_kms_legacy_classify_objects_total", result)
		if !ok || got != 0 {
			t.Fatalf("%s at startup %v ok=%v", result, got, ok)
		}
	}
	cur0, ok := legacyClassifyValue(t, "datasafe_kms_legacy_classify_current_bytes", "")
	if !ok || cur0 != 0 {
		t.Fatalf("gauge at startup %v ok=%v", cur0, ok)
	}
	ObserveLegacyClassify("plaintext")
	SetLegacyClassifyCurrent(32 << 20)
	got, ok := legacyClassifyValue(t, "datasafe_kms_legacy_classify_objects_total", "plaintext")
	if !ok || got != 1 {
		t.Fatalf("plaintext decisions %v ok=%v", got, ok)
	}
	cur, ok := legacyClassifyValue(t, "datasafe_kms_legacy_classify_current_bytes", "")
	if !ok || cur != float64(32<<20) {
		t.Fatalf("current bytes %v ok=%v", cur, ok)
	}
	ResetLegacyClassifyMetricsForTest()
	if _, ok := legacyClassifyValue(t, "datasafe_kms_legacy_classify_objects_total", "plaintext"); ok {
		t.Fatal("counter still registered after reset")
	}
	if _, ok := legacyClassifyValue(t, "datasafe_kms_legacy_classify_current_bytes", ""); ok {
		t.Fatal("gauge still registered after reset")
	}
}

func legacyClassifyValue(t *testing.T, name, result string) (float64, bool) {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if result != "" && !hasResultLabel(m, result) {
				continue
			}
			if m.GetCounter() != nil {
				return m.GetCounter().GetValue(), true
			}
			if m.GetGauge() != nil {
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}

func hasResultLabel(m *dto.Metric, result string) bool {
	for _, l := range m.GetLabel() {
		if l.GetName() == "result" && l.GetValue() == result {
			return true
		}
	}
	return false
}
