package config

import (
	"github.com/pkg/errors"
)

// MetricsCheck is the metrics check type.
type MetricsCheck string

// Supported metrics checks.
const (
	LatencyMetricsCheck   MetricsCheck = "request-latency"
	ErrorRateMetricsCheck MetricsCheck = "error-rate-percent"
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
	Steps          []int64
	HealthCriteria []Metric

	// TODO: Give this property a clearer name.
	Interval int64
}

// Config contains the configuration for the application.
//
// It is the configuration for entire process for all the possible services that
// are selected through the targets field.
type Config struct {
	Targets  []*Target
	Strategy *Strategy
}

// WithValues initializes a configuration with the given values.
func WithValues(targets []*Target, steps []int64, interval int64, metrics []Metric) *Config {
	return &Config{
		Targets: targets,
		Strategy: &Strategy{
			Steps:          steps,
			HealthCriteria: metrics,
			Interval:       interval,
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
func (config *Config) Validate(cliMode bool) error {
	if cliMode && config.Strategy.Interval <= 0 {
		return errors.New("time interval must be greater than 0")
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
	switch metricsCriteria.Type {
	case ErrorRateMetricsCheck:
		if threshold > 100 || threshold < 0 {
			return errors.Errorf("threshold must be greater than 0 and less than 100 for %q", metricsCriteria.Type)
		}
	case LatencyMetricsCheck:
		percentile := metricsCriteria.Percentile
		if percentile != 99 && percentile != 95 && percentile != 50 {
			return errors.Errorf("invalid percentile for %q", metricsCriteria.Type)
		}
		if metricsCriteria.Threshold < 0 {
			return errors.Errorf("threshold cannot be negative for %q", metricsCriteria.Type)
		}
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
