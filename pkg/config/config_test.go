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
			name:     "correct config",
			config:   config.WithValues("myproject", "us-east1", "mysvc", []int64{5, 30, 60}, 20),
			cliMode:  true,
			expected: true,
		},
		// No project.
		{
			name:     "missing project",
			config:   config.WithValues("", "us-east1", "mysvc", []int64{5, 30, 60}, 20),
			cliMode:  true,
			expected: false,
		},
		// No steps
		{
			name:     "missing steps",
			config:   config.WithValues("myproject", "us-east1", "mysvc", []int64{}, 20),
			cliMode:  true,
			expected: false,
		},
		// Steps are not in ascending order.
		{
			name:     "steps not in order",
			config:   config.WithValues("myproject", "us-east1", "mysvc", []int64{30, 30, 60}, 20),
			cliMode:  true,
			expected: false,
		},
		// A step is greater than 100.
		{
			name:     "step greater than 100",
			config:   config.WithValues("myproject", "us-east1", "mysvc", []int64{5, 30, 101}, 20),
			cliMode:  true,
			expected: false,
		},
		// No interval for CLI mode.
		{
			name:     "no interval for cli mode",
			config:   config.WithValues("myproject", "us-east1", "mysvc", []int64{5, 30, 101}, 0),
			cliMode:  true,
			expected: false,
		},
	}

	for _, test := range tests {
		isValid := test.config.IsValid(test.cliMode)

		assert.Equal(t, test.expected, isValid)
	}
}
