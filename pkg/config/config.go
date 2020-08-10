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

// HealthCriterion is a metrics threshold that should be met to consider a
// candidate healthy.
type HealthCriterion struct {
	Metric     MetricsCheck
	Percentile float64
	Threshold  float64
}

// Strategy is a rollout configuration for the targeted services.
type Strategy struct {
	Target              Target
	Steps               []int64
	HealthCriteria      []HealthCriterion
	HealthOffsetMinute  int
	TimeBetweenRollouts time.Duration
}

// Config contains the configuration for the application.
type Config struct {
	Strategies []Strategy
}

// NewTarget initializes a target to filter services by label.
func NewTarget(project string, regions []string, labelSelector string) Target {
	return Target{
		Project:       project,
		Regions:       regions,
		LabelSelector: labelSelector,
	}
}

// NewStrategy initializes a strategy.
func NewStrategy(target Target, steps []int64, healthOffset int, timeBetweenRollouts time.Duration, healthCriteria []HealthCriterion) Strategy {
	return Strategy{
		Target:              target,
		Steps:               steps,
		HealthCriteria:      healthCriteria,
		HealthOffsetMinute:  healthOffset,
		TimeBetweenRollouts: timeBetweenRollouts,
	}
}

// Validate checks if the configuration is valid.
func (config Config) Validate() error {
	for i, strategy := range config.Strategies {
		err := strategy.Validate()
		if err != nil {
			return errors.Wrapf(err, "invalid strategy at index %d", i)
		}
	}
	return nil
}

// Validate checks if the strategy is valid.
func (strategy Strategy) Validate() error {
	if strategy.HealthOffsetMinute <= 0 {
		return errors.Errorf("health check offset must be positive, got %d", strategy.HealthOffsetMinute)
	}

	if len(strategy.Steps) == 0 {
		return errors.New("steps cannot be empty")
	}

	// Steps must be in ascending order and not greater than 100.
	var previous int64
	for _, step := range strategy.Steps {
		if step <= previous || step > 100 {
			return errors.New("steps must be in ascending order and not greater than 100")
		}
		previous = step
	}

	for i, criterion := range strategy.HealthCriteria {
		if err := validateHealthCriterion(criterion); err != nil {
			return errors.Wrapf(err, "invalid metrics criterion at index %d", i)
		}
	}
	return validateTarget(strategy.Target)
}

func validateHealthCriterion(criterion HealthCriterion) error {
	threshold := criterion.Threshold
	if threshold < 0 {
		return errors.Errorf("threshold cannot be negative, criterion %q", criterion.Metric)
	}

	switch criterion.Metric {
	case ErrorRateMetricsCheck:
		if threshold > 100 {
			return errors.Errorf("threshold must be greater than 0 and less than 100 for %q", criterion.Metric)
		}
	case LatencyMetricsCheck:
		percentile := criterion.Percentile
		if percentile != 99 && percentile != 95 && percentile != 50 {
			return errors.Errorf("invalid percentile for %.2f", criterion.Percentile)
		}
	case RequestCountMetricsCheck:
		return nil
	default:
		return errors.Errorf("invalid metric criteria %q", criterion.Metric)
	}

	return nil
}

func validateTarget(target Target) error {
	if target.Project == "" {
		return errors.Errorf("project must be specified")
	}
	if target.LabelSelector == "" {
		return errors.Errorf("label must be specified")
	}
	return nil
}
