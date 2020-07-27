package health_test

import (
	"context"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	metricsMocker "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics/mock"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/health"
	"github.com/stretchr/testify/assert"
)

func TestDiagnose(t *testing.T) {
	metricsMock := &metricsMocker.Metrics{}
	metricsMock.LatencyFn = func(ctx context.Context, query metrics.Query, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
		return 500, nil
	}
	metricsMock.ErrorRateFn = func(ctx context.Context, query metrics.Query, offset time.Duration) (float64, error) {
		return 0.01, nil
	}

	tests := []struct {
		name           string
		query          metrics.Query
		offset         time.Duration
		minRequests    int64
		healthCriteria []config.Metric
		expected       *health.Diagnosis
	}{
		{
			name:        "healthy revision",
			query:       metricsMocker.Query{},
			offset:      5 * time.Minute,
			minRequests: 1000,
			healthCriteria: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Type: config.ErrorRateMetricsCheck, Threshold: 5},
			},
			expected: &health.Diagnosis{
				OverallResult: health.Healthy,
				CheckResults: []health.CheckResult{
					{
						Threshold:     750.0,
						ActualValue:   500.0,
						IsCriteriaMet: true,
					},
					{
						Threshold:     5.0,
						ActualValue:   1.0,
						IsCriteriaMet: true,
					},
				},
			},
		},
		{
			name:   "barely healthy revision",
			query:  metricsMocker.Query{},
			offset: 5 * time.Minute,
			healthCriteria: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 500},
				{Type: config.ErrorRateMetricsCheck, Threshold: 1},
			},
			expected: &health.Diagnosis{
				OverallResult: health.Healthy,
				CheckResults: []health.CheckResult{
					{
						Threshold:     500.0,
						ActualValue:   500.0,
						IsCriteriaMet: true,
					},
					{
						Threshold:     1.0,
						ActualValue:   1.0,
						IsCriteriaMet: true,
					},
				},
			},
		},
		{
			name:        "unhealthy revision, miss latency",
			query:       metricsMocker.Query{},
			offset:      5 * time.Minute,
			minRequests: 1000,
			healthCriteria: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 499},
			},
			expected: &health.Diagnosis{
				OverallResult: health.Unhealthy,
				CheckResults: []health.CheckResult{
					{
						Threshold:     499.0,
						ActualValue:   500.0,
						IsCriteriaMet: false,
					},
				},
			},
		},
		{
			name:        "unhealthy revision, miss error rate",
			query:       metricsMocker.Query{},
			offset:      5 * time.Minute,
			minRequests: 1000,
			healthCriteria: []config.Metric{
				{Type: config.ErrorRateMetricsCheck, Threshold: 0.95},
			},
			expected: &health.Diagnosis{
				OverallResult: health.Unhealthy,
				CheckResults: []health.CheckResult{
					{
						Threshold:     0.95,
						ActualValue:   1.0,
						IsCriteriaMet: false,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			diagnosis, _ := health.Diagnose(ctx, metricsMock, test.query, test.offset, test.minRequests, test.healthCriteria)
			assert.Equal(t, test.expected, diagnosis)
		})
	}
}
