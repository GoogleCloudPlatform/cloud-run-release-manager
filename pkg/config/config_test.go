package config_test

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.Config
		cliMode  bool
		expected bool
	}{
		{
			name: "correct config with label selector",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20),
			cliMode:  true,
			expected: true,
		},
		// No project.
		{
			name: "missing project",
			config: config.WithValues([]*config.Target{
				config.NewTarget("", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20),
			cliMode:  true,
			expected: false,
		},
		// No steps
		{
			name: "missing steps",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{}, 20),
			cliMode:  true,
			expected: false,
		},
		// Steps are not in ascending order.
		{
			name: "steps not in order",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{30, 30, 60}, 20),
			cliMode:  true,
			expected: false,
		},
		// A step is greater than 100.
		{
			name: "step greater than 100",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 101}, 20),
			cliMode:  true,
			expected: false,
		},
		// No interval for CLI mode.
		{
			name: "no interval for cli mode",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 0),
			cliMode:  true,
			expected: false,
		},
		// Empty label selector.
		{
			name: "empty label selector",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, ""),
			}, []int64{5, 30, 60}, 20),
			cliMode:  true,
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isValid := test.config.IsValid(test.cliMode)

			assert.Equal(t, test.expected, isValid)
		})
	}
}
