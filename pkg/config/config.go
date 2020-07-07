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
