package rollout

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNextCandidateTraffic100(t *testing.T) {
	strategy := config.Strategy{
		Steps: []int64{5, 30, 60},
	}
	r := &Rollout{strategy: strategy}

	var tests = []struct {
		in  int64
		out int64
	}{
		{0, 5},
		{5, 30},
		{10, 30},
		{30, 60},
		{59, 60},
		{60, 100},
		{100, 100},
	}

	for _, test := range tests {
		next := r.nextCandidateTraffic(test.in)
		assert.Equal(t, test.out, next)
	}
}
