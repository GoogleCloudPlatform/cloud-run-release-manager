package config

// Metadata is the information on the service to be managed.
type Metadata struct {
	Project string `json:"project" yaml:"project"`
	Service string `json:"service" yaml:"service"`
	Region  string `json:"region"  yaml:"region"`
}

// Rollout is the steps and configuration for rollout.
type Rollout struct {
	Steps    []int64 `json:"steps" yaml:"steps"`
	Interval int64   `yaml:"interval"`
}

// Config contains the configuration for a managed rollout.
type Config struct {
	Metadata *Metadata `json:"metadata" yaml:"metadata"`
	Rollout  *Rollout  `json:"rollout" yaml:"rollout"`
}

// WithValues initializes a configuration with the given values.
func WithValues(project, region, service string, steps []int64, interval int64) *Config {
	return &Config{
		Metadata: &Metadata{
			Project: project,
			Region:  region,
			Service: service,
		},
		Rollout: &Rollout{
			Steps:    steps,
			Interval: interval,
		},
	}
}

// IsValid checks if the configuration is valid.
func (config *Config) IsValid(cliMode bool) bool {
	if config.Metadata.Project == "" ||
		config.Metadata.Service == "" ||
		config.Metadata.Region == "" {

		return false
	}

	if cliMode && config.Rollout.Interval <= 0 {
		return false
	}

	if len(config.Rollout.Steps) == 0 {
		return false
	}

	// Steps must be in ascending order and not greater than 100.
	var previous int64
	for _, step := range config.Rollout.Steps {
		if step <= previous || step > 100 {
			return false
		}
		previous = step
	}

	return true
}
