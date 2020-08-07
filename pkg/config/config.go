package config

import (
	"time"

	"github.com/pkg/errors"
)

// MetricsCheck is the metrics check type.
type MetricsCheck string

// Supported metrics checks.
const (
	RequestCountMetricsCheck MetricsCheck = "request-count"
	LatencyMetricsCheck      MetricsCheck = "request-latency"
	ErrorRateMetricsCheck    MetricsCheck = "error-rate-percent"
)

// Target is the configuration to filter services.
//
// A target might have the following form
//
// {
//   "project": "myproject"
//   "regions": [us-east1, us-central1]
//   "labelSelector": "team=backend"
// }
type Target struct {
	Project       string
	Regions       []string
	LabelSelector string
}

// Metric is a metrics threshold that should be met to consider a candidate
// healthy.
type Metric struct {
	Type       MetricsCheck
	Percentile float64
	Threshold  float64
}

// Strategy is the steps and configuration for rollout.
type Strategy struct {
	Steps               []int64
	HealthCriteria      []Metric
	HealthOffsetMinute  int
	TimeBetweenRollouts time.Duration
}

// Config contains the configuration for the application.
//
// It is the configuration for entire process for all the possible services that
// are selected through the targets field.
type Config struct {
	Targets  []*Target
	Strategy *Strategy
}

// New initializes a configuration.
func New(targets []*Target, steps []int64, healthOffset int, timeBetweenRollouts time.Duration, metrics []Metric) *Config {
	return &Config{
		Targets: targets,
		Strategy: &Strategy{
			Steps:               steps,
			HealthCriteria:      metrics,
			HealthOffsetMinute:  healthOffset,
			TimeBetweenRollouts: timeBetweenRollouts,
		},
	}
}

// NewTarget initializes a target to filter services by label.
func NewTarget(project string, regions []string, labelSelector string) *Target {
	return &Target{
		Project:       project,
		Regions:       regions,
		LabelSelector: labelSelector,
	}
}

// Validate checks if the configuration is valid.
func (config *Config) Validate() error {
	if config.Strategy.HealthOffsetMinute <= 0 {
		return errors.Errorf("health check offset must be positive, got %d", config.Strategy.HealthOffsetMinute)
	}

	if len(config.Strategy.Steps) == 0 {
		return errors.New("steps cannot be empty")
	}

	// Steps must be in ascending order and not greater than 100.
	var previous int64
	for _, step := range config.Strategy.Steps {
		if step <= previous || step > 100 {
			return errors.New("steps must be in ascending order and not greater than 100")
		}
		previous = step
	}

	for i, criteria := range config.Strategy.HealthCriteria {
		if err := validateMetrics(criteria); err != nil {
			return errors.Wrapf(err, "invalid metrics criteria at index %d", i)
		}
	}
	return validateTargets(config.Targets)
}

func validateMetrics(metricsCriteria Metric) error {
	threshold := metricsCriteria.Threshold
	if threshold < 0 {
		return errors.Errorf("threshold cannot be negative, criteria %q", metricsCriteria.Type)
	}

	switch metricsCriteria.Type {
	case ErrorRateMetricsCheck:
		if threshold > 100 {
			return errors.Errorf("threshold must be greater than 0 and less than 100 for %q", metricsCriteria.Type)
		}
	case LatencyMetricsCheck:
		percentile := metricsCriteria.Percentile
		if percentile != 99 && percentile != 95 && percentile != 50 {
			return errors.Errorf("invalid percentile for %q", metricsCriteria.Type)
		}
	case RequestCountMetricsCheck:
		return nil
	default:
		return errors.Errorf("invalid metric criteria %q", metricsCriteria.Type)
	}

	return nil
}

func validateTargets(targets []*Target) error {
	for i, target := range targets {
		if target.Project == "" {
			return errors.Errorf("project must be specified in target at index %d", i)
		}

		if target.LabelSelector == "" {
			return errors.Errorf("label must be specified in target at index %d", i)
		}
	}

	return nil
}
