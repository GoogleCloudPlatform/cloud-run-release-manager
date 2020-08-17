package health

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestIsCriteriaMet(t *testing.T) {
	tests := []struct {
		name        string
		metricsType config.MetricsCheck
		threshold   float64
		actualValue float64
		expected    bool
	}{
		{
			name:        "met request count",
			metricsType: config.RequestCountMetricsCheck,
			threshold:   1000,
			actualValue: 1000,
			expected:    true,
		},
		{
			name:        "met latency",
			metricsType: config.LatencyMetricsCheck,
			threshold:   750,
			actualValue: 500,
			expected:    true,
		},
		{
			name:        "met error rate",
			metricsType: config.ErrorRateMetricsCheck,
			threshold:   1,
			actualValue: 1,
			expected:    true,
		},
		{
			name:        "unmet request count",
			metricsType: config.RequestCountMetricsCheck,
			threshold:   1000,
			actualValue: 700,
			expected:    false,
		},
		{
			name:        "unmet latency",
			metricsType: config.LatencyMetricsCheck,
			threshold:   750,
			actualValue: 751,
			expected:    false,
		},
		{
			name:        "unmet error rate",
			metricsType: config.ErrorRateMetricsCheck,
			threshold:   1,
			actualValue: 1.01,
			expected:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			isMet := isCriteriaMet(test.metricsType, test.threshold, test.actualValue)
			assert.Equal(tt, test.expected, isMet)
		})
	}
}
