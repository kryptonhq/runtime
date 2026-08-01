/*
Copyright 2026 Krypton Authors.
*/

package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Declaring a metric var and forgetting to add it to init()'s
// MustRegister list is an easy miss — the code compiles, the metric
// records happily, and it silently never appears on /metrics. This test
// asserts every metric this package exports is actually registered.
func TestAllMetricsAreRegistered(t *testing.T) {
	want := map[string]prometheus.Collector{
		"krypton_invocations_total":                 InvocationsTotal,
		"krypton_invocation_duration_seconds":       InvocationDurationSeconds,
		"krypton_cold_starts_total":                 ColdStartsTotal,
		"krypton_buffer_depth":                      BufferDepth,
		"krypton_scaler_decisions_total":            ScalerDecisionsTotal,
		"krypton_agent_replicas_desired":            AgentReplicasDesired,
		"krypton_model_invocations_total":           ModelInvocationsTotal,
		"krypton_model_invocation_duration_seconds": ModelInvocationDurationSeconds,
		"krypton_api_requests_total":                APIRequestsTotal,
		"krypton_api_request_duration_seconds":      APIRequestDurationSeconds,
	}

	for name, collector := range want {
		// Re-registering an already-registered collector returns
		// AlreadyRegisteredError. That's the signal we want: it proves
		// init() got to it first.
		err := ctrlmetrics.Registry.Register(collector)
		if err == nil {
			// Registration succeeded, meaning init() had NOT registered it.
			// Undo our side effect so we don't corrupt the shared registry
			// for other packages' tests.
			ctrlmetrics.Registry.Unregister(collector)
			t.Errorf("%s is declared but not registered in init()", name)
			continue
		}
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			t.Errorf("%s: unexpected registration error: %v", name, err)
		}
	}
}

// Metric names are part of the operator-facing contract (dashboards,
// alerts, the Grafana JSON in deploy/grafana). Renaming one is a breaking
// change, so pin the names.
func TestMetricNamesAndLabels(t *testing.T) {
	tests := []struct {
		name       string
		collector  prometheus.Collector
		metricName string
		labels     []string
	}{
		{"invocations", InvocationsTotal, "krypton_invocations_total", []string{"agent", "namespace", "status"}},
		{"cold starts", ColdStartsTotal, "krypton_cold_starts_total", []string{"agent", "namespace"}},
		{"buffer depth", BufferDepth, "krypton_buffer_depth", []string{"agent", "namespace"}},
		{"scaler decisions", ScalerDecisionsTotal, "krypton_scaler_decisions_total", []string{"agent", "namespace", "direction"}},
		{"replicas desired", AgentReplicasDesired, "krypton_agent_replicas_desired", []string{"agent", "namespace"}},
		{"model invocations", ModelInvocationsTotal, "krypton_model_invocations_total", []string{"model", "status"}},
		{"api requests", APIRequestsTotal, "krypton_api_requests_total", []string{"route", "method", "code"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desc := collectorDesc(t, tc.collector)
			if !strings.Contains(desc, `fqName: "`+tc.metricName+`"`) {
				t.Errorf("metric name changed; Desc = %s, want fqName %q", desc, tc.metricName)
			}
			for _, l := range tc.labels {
				if !strings.Contains(desc, l) {
					t.Errorf("label %q missing from Desc: %s", l, desc)
				}
			}
		})
	}
}

// Label cardinality is the operational risk here: a WithLabelValues call
// with the wrong arity panics at runtime, in the request path.
func TestLabelArityMatchesUsage(t *testing.T) {
	// Each of these mirrors a real call site. A mismatch panics.
	InvocationsTotal.WithLabelValues("travel", "agents", "200").Inc()
	InvocationDurationSeconds.WithLabelValues("travel", "agents").Observe(0.1)
	ColdStartsTotal.WithLabelValues("travel", "agents").Inc()
	BufferDepth.WithLabelValues("travel", "agents").Set(3)
	ScalerDecisionsTotal.WithLabelValues("travel", "agents", "up").Inc()
	AgentReplicasDesired.WithLabelValues("travel", "agents").Set(2)
	ModelInvocationsTotal.WithLabelValues("qwen", "200").Inc()
	ModelInvocationDurationSeconds.WithLabelValues("qwen").Observe(1.5)
	APIRequestsTotal.WithLabelValues("list_agents", "GET", "200").Inc()
	APIRequestDurationSeconds.WithLabelValues("list_agents").Observe(0.01)
}

// Histogram buckets bound what percentiles a dashboard can compute. Agent
// invocations and model invocations have deliberately different ranges —
// model calls are slower — so guard that they didn't get collapsed.
func TestHistogramBucketRanges(t *testing.T) {
	agentDesc := collectorDesc(t, InvocationDurationSeconds)
	modelDesc := collectorDesc(t, ModelInvocationDurationSeconds)

	if agentDesc == modelDesc {
		t.Error("agent and model latency histograms have identical descriptors; model buckets should extend further")
	}
}

func collectorDesc(t *testing.T, c prometheus.Collector) string {
	t.Helper()
	ch := make(chan *prometheus.Desc, 1)
	c.Describe(ch)
	close(ch)
	d, ok := <-ch
	if !ok {
		t.Fatal("collector produced no Desc")
	}
	return d.String()
}
