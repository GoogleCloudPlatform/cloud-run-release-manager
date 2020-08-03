package config_test

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		cliMode   bool
		shouldErr bool
	}{
		{
			name: "correct config with label selector",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20, nil),
		},
		{
			name: "missing project",
			config: config.WithValues([]*config.Target{
				config.NewTarget("", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20, nil),
			shouldErr: true,
		},
		{
			name: "missing steps",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{}, 20, nil),
			cliMode:   true,
			shouldErr: true,
		},
		{
			name: "steps not in order",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{30, 30, 60}, 20, nil),
			cliMode:   true,
			shouldErr: true,
		},
		{
			name: "step greater than 100",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 101}, 20, nil),
			shouldErr: true,
		},
		{
			name: "no interval for cli mode",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 0, nil),
			cliMode:   true,
			shouldErr: true,
		},
		{
			name: "empty label selector",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, ""),
			}, []int64{5, 30, 60}, 20, nil),
			shouldErr: true,
		},
		{
			name: "invalid request count value",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20,
				[]config.Metric{
					{Type: config.RequestCountMetricsCheck, Threshold: -1},
				},
			),
			shouldErr: true,
		},
		{
			name: "invalid error rate in metrics",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20,
				[]config.Metric{
					{Type: config.ErrorRateMetricsCheck, Threshold: 101},
				},
			),
			shouldErr: true,
		},
		{
			name: "invalid latency percentile",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20,
				[]config.Metric{
					{Type: config.LatencyMetricsCheck, Percentile: 98},
				},
			),
			shouldErr: true,
		},
		{
			name: "invalid latency value",
			config: config.WithValues([]*config.Target{
				config.NewTarget("myproject", []string{"us-east1", "us-west1"}, "team=backend"),
			}, []int64{5, 30, 60}, 20,
				[]config.Metric{
					{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: -1},
				},
			),
			shouldErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate(test.cliMode)
			if test.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
