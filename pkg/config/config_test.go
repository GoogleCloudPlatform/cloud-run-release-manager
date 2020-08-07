package config_test

import (
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		name                string
		targets             []*config.Target
		steps               []int64
		healthOffset        int
		timeBetweenRollouts time.Duration
		metrics             []config.Metric
		shouldErr           bool
	}{
		{
			name: "correct config with label selector",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Type: config.RequestCountMetricsCheck, Threshold: 1000},
			},
			shouldErr: false,
		},
		{
			name: "missing project",
			targets: []*config.Target{
				config.NewTarget("", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics:             nil,
			shouldErr:           true,
		},
		{
			name: "missing steps",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics:             nil,
			shouldErr:           true,
		},
		{
			name: "steps not in order",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{30, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics:             nil,
			shouldErr:           true,
		},
		{
			name: "step greater than 100",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 101},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics:             nil,
			shouldErr:           true,
		},
		{
			name: "non-positive health offset",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        0,
			timeBetweenRollouts: 10 * time.Minute,
			metrics:             nil,
			shouldErr:           true,
		},
		{
			name: "empty label selector",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, ""),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics:             nil,
			shouldErr:           true,
		},
		{
			name: "invalid request count value",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics: []config.Metric{
				{Type: config.RequestCountMetricsCheck, Threshold: -1},
			},
			shouldErr: true,
		},
		{
			name: "invalid error rate in metrics",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics: []config.Metric{
				{Type: config.ErrorRateMetricsCheck, Threshold: 101},
			},
			shouldErr: true,
		},
		{
			name: "invalid latency percentile",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 98},
			},
			shouldErr: true,
		},
		{
			name: "invalid latency value",
			targets: []*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			},
			steps:               []int64{5, 30, 60},
			healthOffset:        20,
			timeBetweenRollouts: 10 * time.Minute,
			metrics: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: -1},
			},
			shouldErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			config := config.New(test.targets, test.steps, test.healthOffset, test.timeBetweenRollouts, test.metrics)
			err := config.Validate()
			if test.shouldErr {
				assert.NotNil(tt, err)
			} else {
				assert.Nil(tt, err)
			}
		})
	}
}
