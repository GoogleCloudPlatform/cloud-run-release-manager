package health_test

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/health"
	"github.com/stretchr/testify/assert"
)

func TestStringReport(t *testing.T) {
	tests := []struct {
		name           string
		healthCriteria []config.HealthCriterion
		diagnosis      health.Diagnosis
		expected       string
	}{
		{
			name: "single metrics",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
			},
			diagnosis: health.Diagnosis{
				OverallResult: health.Unhealthy,
				CheckResults: []health.CheckResult{
					{Threshold: 750, ActualValue: 1000, IsCriteriaMet: true},
				},
			},
			expected: "status: unhealthy\n" +
				"metrics:" +
				"\n- request-latency[p99]: 1000.00 (needs 750.00)",
		},
		{
			name: "more than one metrics",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.RequestCountMetricsCheck, Threshold: 1000},
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Metric: config.ErrorRateMetricsCheck, Threshold: 5},
			},
			diagnosis: health.Diagnosis{
				OverallResult: health.Healthy,
				CheckResults: []health.CheckResult{
					{Threshold: 1000, ActualValue: 1500, IsCriteriaMet: true},
					{Threshold: 750, ActualValue: 500, IsCriteriaMet: true},
					{Threshold: 5, ActualValue: 2, IsCriteriaMet: true},
				},
			},
			expected: "status: healthy\n" +
				"metrics:" +
				"\n- request-count: 1500 (needs 1000)" +
				"\n- request-latency[p99]: 500.00 (needs 750.00)" +
				"\n- error-rate-percent: 2.00 (needs 5.00)",
		},
		{
			name:     "no metrics",
			expected: "status: unknown\nmetrics:",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			report := health.StringReport(test.healthCriteria, test.diagnosis)
			assert.Equal(tt, test.expected, report)
		})
	}
}
