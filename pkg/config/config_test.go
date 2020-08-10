package config_test

import (
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestStrategy_Validate(t *testing.T) {
	tests := []struct {
		name                string
		target              config.Target
		steps               []int64
		healthOffset        int
		timeBetweenRollouts time.Duration
		healthCriteria      []config.HealthCriterion
		shouldErr           bool
	}{
		{
			name:                "correct config with label selector",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Metric: config.RequestCountMetricsCheck, Threshold: 1000},
			},
			shouldErr: false,
		},
		{
			name:                "missing project",
			target:              config.NewTarget("", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			healthCriteria:      nil,
			shouldErr:           true,
		},
		{
			name:                "missing steps",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			shouldErr:           true,
		},
		{
			name:                "steps not in order",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{30, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			shouldErr:           true,
		},
		{
			name:                "step greater than 100",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 101},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			shouldErr:           true,
		},
		{
			name:                "non-positive health offset",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 60},
			healthOffset:        0,
			timeBetweenRollouts: 10 * time.Minute,
			shouldErr:           true,
		},
		{
			name:                "empty label selector",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, ""),
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			shouldErr:           true,
		},
		{
			name:                "invalid request count value",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			healthCriteria: []config.HealthCriterion{
				{Metric: config.RequestCountMetricsCheck, Threshold: -1},
			},
			shouldErr: true,
		},
		{
			name:                "invalid error rate in criteria",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			healthCriteria: []config.HealthCriterion{
				{Metric: config.ErrorRateMetricsCheck, Threshold: 101},
			},
			shouldErr: true,
		},
		{
			name:                "invalid latency percentile",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 98},
			},
			shouldErr: true,
		},
		{
			name:                "invalid latency value",
			target:              config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			healthCriteria: []config.HealthCriterion{
				{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: -1},
			},
			shouldErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			strategy := config.NewStrategy(test.target, test.steps, test.healthOffset, test.timeBetweenRollouts, test.healthCriteria)
			err := strategy.Validate()
			if test.shouldErr {
				assert.NotNil(tt, err)
			} else {
				assert.Nil(tt, err)
			}
		})
	}
}
