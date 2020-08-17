package health_test

import (
	"context"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/health"
	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/metrics"
	metricsMocker "github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/metrics/mock"
	"github.com/stretchr/testify/assert"
)

func TestDiagnosis(t *testing.T) {
	tests := []struct {
		name           string
		healthCriteria []config.HealthCriterion
		results        []float64
		expected       health.Diagnosis
		shouldErr      bool
	}{
		{
			name: "healthy revision",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Metric: config.ErrorRateMetricsCheck, Threshold: 5},
			},
			results: []float64{500.0, 1.0},
			expected: health.Diagnosis{
				OverallResult: health.Healthy,
				CheckResults: []health.CheckResult{
					{Threshold: 750.0, ActualValue: 500.0, IsCriteriaMet: true},
					{Threshold: 5.0, ActualValue: 1.0, IsCriteriaMet: true},
				},
			},
		},
		{
			name: "barely healthy revision",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 500},
				{Metric: config.ErrorRateMetricsCheck, Threshold: 1},
			},
			results: []float64{500.0, 1.0},
			expected: health.Diagnosis{
				OverallResult: health.Healthy,
				CheckResults: []health.CheckResult{
					{Threshold: 500.0, ActualValue: 500.0, IsCriteriaMet: true},
					{Threshold: 1.0, ActualValue: 1.0, IsCriteriaMet: true},
				},
			},
		},
		{
			name: "no enough requests, inconclusive",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.RequestCountMetricsCheck, Threshold: 1000},
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 500},
			},
			results: []float64{800, 750.0},
			expected: health.Diagnosis{
				OverallResult: health.Inconclusive,
				CheckResults:  nil,
			},
		},
		{
			name: "only request count criteria, unknown",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.RequestCountMetricsCheck, Threshold: 1000},
			},
			results: []float64{1500},
			expected: health.Diagnosis{
				OverallResult: health.Unknown,
				CheckResults: []health.CheckResult{
					{Threshold: 1000, ActualValue: 1500, IsCriteriaMet: true},
				},
			},
		},
		{
			name: "unhealthy revision, miss latency",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 499},
			},
			results: []float64{500.0},
			expected: health.Diagnosis{
				OverallResult: health.Unhealthy,
				CheckResults: []health.CheckResult{
					{Threshold: 499.0, ActualValue: 500.0},
				},
			},
		},
		{
			name: "unhealthy revision, miss error rate",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.ErrorRateMetricsCheck, Threshold: 0.95},
			},
			results: []float64{1.0},
			expected: health.Diagnosis{
				OverallResult: health.Unhealthy,
				CheckResults: []health.CheckResult{
					{Threshold: 0.95, ActualValue: 1.0},
				},
			},
		},
		{
			name: "zero threshold",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 0},
				{Metric: config.ErrorRateMetricsCheck, Threshold: 0},
			},
			results: []float64{500.0, 1.0},
			expected: health.Diagnosis{
				OverallResult: health.Unhealthy,
				CheckResults: []health.CheckResult{
					{Threshold: 0, ActualValue: 500.0, IsCriteriaMet: false},
					{Threshold: 0, ActualValue: 1.0, IsCriteriaMet: false},
				},
			},
		},
		{
			name: "zero metrics values",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Metric: config.ErrorRateMetricsCheck, Threshold: 5},
			},
			results: []float64{0, 0},
			expected: health.Diagnosis{
				OverallResult: health.Healthy,
				CheckResults: []health.CheckResult{
					{Threshold: 750, ActualValue: 0, IsCriteriaMet: true},
					{Threshold: 5, ActualValue: 0, IsCriteriaMet: true},
				},
			},
		},
		{
			name: "should err, different sizes for criteria and results",
			healthCriteria: []config.HealthCriterion{
				{Metric: config.ErrorRateMetricsCheck, Threshold: 0.95},
			},
			results:   []float64{},
			shouldErr: true,
		},
		{
			name:      "should err, empty health criteria",
			shouldErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			ctx := context.Background()
			diagnosis, err := health.Diagnose(ctx, test.healthCriteria, test.results)
			if test.shouldErr {
				assert.NotNil(tt, err)
			} else {
				assert.Equal(tt, test.expected, diagnosis)
			}
		})
	}
}

// TestCollectMetrics tests that health.CollectMetrics returns values using the
// metrics provider.
func TestCollectMetrics(t *testing.T) {
	metricsMock := &metricsMocker.Metrics{}
	metricsMock.RequestCountFn = func(ctx context.Context, offset time.Duration) (int64, error) {
		return 1000, nil
	}
	metricsMock.LatencyFn = func(ctx context.Context, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
		return 500, nil
	}
	metricsMock.ErrorRateFn = func(ctx context.Context, offset time.Duration) (float64, error) {
		return 0.01, nil
	}

	ctx := context.Background()
	offset := 5 * time.Minute
	healthCriteria := []config.HealthCriterion{
		{Metric: config.RequestCountMetricsCheck},
		{Metric: config.LatencyMetricsCheck, Percentile: 99},
		{Metric: config.ErrorRateMetricsCheck},
	}
	expected := []float64{1000, 500.0, 1.0}

	results, err := health.CollectMetrics(ctx, metricsMock, offset, healthCriteria)
	assert.Nil(t, err)
	assert.Equal(t, expected, results)
}
