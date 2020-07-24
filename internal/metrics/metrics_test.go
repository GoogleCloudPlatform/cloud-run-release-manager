package metrics_test

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestPercentileToAlignReduce(t *testing.T) {
	tests := []struct {
		name       string
		percentile float64
		expected   metrics.AlignReduce
		shouldErr  bool
	}{
		{
			name:       "99.0 (99th percentile)",
			percentile: 99.0,
			expected:   metrics.Align99Reduce99,
		},
		{
			name:       "99 (99th percentile)",
			percentile: 99,
			expected:   metrics.Align99Reduce99,
		},
		{
			name:       "99.01 (invalid)",
			percentile: 99.01,
			shouldErr:  true,
		},
		{
			name:       "95.0 (95th percentile)",
			percentile: 95.0,
			expected:   metrics.Align95Reduce95,
		},
		{
			name:       "95.01 (invalid)",
			percentile: 95.01,
			shouldErr:  true,
		},
		{
			name:       "50 (50th percentile)",
			percentile: 50,
			expected:   metrics.Align50Reduce50,
		},
		{
			name:       "50.01 (invalid)",
			percentile: 50.01,
			shouldErr:  true,
		},
		{
			name:       "49.99999 (invalid)",
			percentile: 49.99999,
			shouldErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			alignReducer, err := metrics.PercentileToAlignReduce(test.percentile)
			if test.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, test.expected, alignReducer)
			}
		})
	}
}
