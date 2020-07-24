package config

// MetricsCheck is the metrics check type.
type MetricsCheck string

// Supported metrics checks.
const (
	LatencyMetricsCheck   MetricsCheck = "request-latency"
	ErrorRateMetricsCheck              = "error-rate-percent"
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
	Steps   []int64
	Metrics []Metric

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
			Steps:    steps,
			Metrics:  metrics,
			Interval: interval,
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

// IsValid checks if the configuration is valid.
func (config *Config) IsValid(cliMode bool) bool {

	if cliMode && config.Strategy.Interval <= 0 {
		return false
	}

	if len(config.Strategy.Steps) == 0 {
		return false
	}

	// Steps must be in ascending order and not greater than 100.
	var previous int64
	for _, step := range config.Strategy.Steps {
		if step <= previous || step > 100 {
			return false
		}
		previous = step
	}

	return targetsAreValid(config.Targets)
}

func targetsAreValid(targets []*Target) bool {
	for _, target := range targets {
		if target.Project == "" {
			return false
		}

		if target.LabelSelector == "" {
			return false
		}
	}

	return true
}
