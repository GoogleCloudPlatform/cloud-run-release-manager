package config

// TargetSelector is the filter to get services.
//
// Type can be either "serviceName" or "labelSelector"
type TargetSelector struct {
	Type   string
	Filter string
}

// The selector types
const (
	LabelSelectorType = "labelSelector"
	ServiceNameType   = "serviceName"
)

// Target is the configuration to filter services.
//
// A target might have the following form
//
// {
//   "project": "myproject"
//   "regions": [us-east1, us-central1]
//   "selector": {
//       "type": "labelSelector"
//       "filter": "team=backend"
//    }
// }
//
// This allows more flexibility to add other types of filtering such as
// filtering by service name:
//
// {
//   "project": "myproject"
//   "regions": [us-east1, us-central1]
//   "selector": {
//       "type": "serviceName"
//       "filter": "mysvc"
//    }
// }
type Target struct {
	Project  string
	Regions  []string
	Selector TargetSelector
}

// Strategy is the steps and configuration for rollout.
type Strategy struct {
	Steps    []int64
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
func WithValues(targets []*Target, steps []int64, interval int64) *Config {
	return &Config{
		Targets: targets,
		Strategy: &Strategy{
			Steps:    steps,
			Interval: interval,
		},
	}
}

// NewTargetForLabelSelector initializes a target to filter services by label.
func NewTargetForLabelSelector(project string, regions []string, labelSelector string) *Target {
	return &Target{
		Project: project,
		Regions: regions,
		Selector: TargetSelector{
			Type:   "labelSelector",
			Filter: labelSelector,
		},
	}
}

// NewTargetForServiceName initializes a target to filter by service name.
func NewTargetForServiceName(project string, regions []string, service string) *Target {
	return &Target{
		Project: project,
		Regions: regions,
		Selector: TargetSelector{
			Type:   "serviceName",
			Filter: service,
		},
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

		if target.Selector.Type != "serviceName" && target.Selector.Type != "labelSelector" {
			return false
		}

		if target.Selector.Filter == "" {
			return false
		}
	}

	return true
}
