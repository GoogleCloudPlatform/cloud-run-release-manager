package health

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/util"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// DiagnosisResult is a possible result after a diagnosis.
type DiagnosisResult int

// Possible diagnosis results.
const (
	Unknown DiagnosisResult = iota
	Inconclusive
	Healthy
	Unhealthy
)

// Diagnosis is the information about the health of the revision.
type Diagnosis struct {
	OverallResult DiagnosisResult
	CheckResults  []CheckResult
}

// CheckResult is information about a metrics criteria check.
type CheckResult struct {
	Threshold     float64
	ActualValue   float64
	IsCriteriaMet bool
}

// Diagnose attempts to determine the health of a revision.
//
// If the minimum number of requests is not met, then health cannot be
// determined and Diagnosis.EnoughRequests is set to false.
//
// Otherwise, all metrics criteria are checked to determine if the revision is
// healthy.
func Diagnose(ctx context.Context, provider metrics.Provider, offset time.Duration,
	minRequests int64, healthCriteria []config.Metric) (*Diagnosis, error) {

	logger := util.LoggerFromContext(ctx)
	metricsValues, err := CollectMetrics(ctx, provider, offset, healthCriteria)
	if err != nil {
		return nil, errors.Wrap(err, "could not collect metrics")
	}

	overallResult := Healthy
	var results []CheckResult
	for i, criteria := range healthCriteria {
		logger := logger.WithFields(logrus.Fields{
			"metricsType": criteria.Type,
			"percentile":  criteria.Percentile,
			"threshold":   criteria.Threshold,
			"actualValue": metricsValues[i],
		})

		result := determineResult(criteria.Type, criteria.Threshold, metricsValues[i])
		results = append(results, result)
		if result.IsCriteriaMet {
			logger.Debug("met criteria")
			continue
		}

		overallResult = Unhealthy
		logger.Debug("unmet criteria")
	}

	return &Diagnosis{
		OverallResult: overallResult,
		CheckResults:  results,
	}, nil
}

// CollectMetrics returns an array of values collected for each of the specified
// metrics criteria.
func CollectMetrics(ctx context.Context, provider metrics.Provider, offset time.Duration, healthCriteria []config.Metric) ([]float64, error) {
	logger := util.LoggerFromContext(ctx)
	logger.Debug("start collecting metrics")
	var values []float64
	for _, criteria := range healthCriteria {
		var value float64
		var err error

		switch criteria.Type {
		case config.LatencyMetricsCheck:
			value, err = latency(ctx, provider, offset, criteria.Percentile)
			break
		case config.ErrorRateMetricsCheck:
			value, err = errorRatePercent(ctx, provider, offset)
			break
		default:
			return nil, errors.Errorf("unimplemented metrics %q", criteria.Type)
		}

		if err != nil {
			return nil, errors.Wrapf(err, "failed to obtain metrics %q", criteria.Type)
		}
		values = append(values, value)
	}

	return values, nil
}

// determineResult concludes if metrics criteria was met.
//
// The returned value also includes a string with details of why the criteria
// was met or not.
func determineResult(metricsType config.MetricsCheck, threshold float64, actualValue float64) CheckResult {
	result := CheckResult{ActualValue: actualValue, Threshold: threshold}

	// As of now, the supported health criteria (latency and error rate) need to
	// be less than the threshold. So, this is sufficient for now but might need
	// to change to a switch statement when criteria with a minimum threshold is
	// added.
	if actualValue <= threshold {
		result.IsCriteriaMet = true
	}
	return result
}

// latency returns the latency for the given offset and percentile.
func latency(ctx context.Context, provider metrics.Provider, offset time.Duration, percentile float64) (float64, error) {
	alignerReducer, err := metrics.PercentileToAlignReduce(percentile)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse percentile")
	}

	logger := util.LoggerFromContext(ctx).WithField("percentile", percentile)
	logger.Debug("querying for latency metrics")
	latency, err := provider.Latency(ctx, offset, alignerReducer)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get latency metrics")
	}
	logger.WithField("value", latency).Debug("latency successfully retrieved")

	return latency, nil
}

// errorRatePercent returns the percentage of errors during the given offset.
func errorRatePercent(ctx context.Context, provider metrics.Provider, offset time.Duration) (float64, error) {
	logger := util.LoggerFromContext(ctx)
	logger.Debug("querying for error rate metrics")
	rate, err := provider.ErrorRate(ctx, offset)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get error rate metrics")
	}

	// Multiply rate by 100 to have a percentage.
	rate *= 100
	logger.WithField("value", rate).Debug("error rate successfully retrieved")
	return rate, nil
}
