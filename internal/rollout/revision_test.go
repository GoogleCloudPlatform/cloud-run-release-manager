package rollout_test

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/rollout"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/run/v1"
)

func TestDetectStableRevisionName(t *testing.T) {
	var tests = []struct {
		name     string
		traffic  []*run.TrafficTarget
		expected string
	}{
		// There's no a stable revision nor a revision handling all the traffic.
		{
			name: "no stable revision",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 50},
				{RevisionName: "test-002", Percent: 50},
			},
			expected: "",
		},
		// There is no stable tag but there's a revision handling all the traffic.
		{
			name: "no stable tag but revision with 100% traffic",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Tag: "new"},
				{RevisionName: "test-001", Percent: 100},
			},
			expected: "test-001",
		},
		// There's a revision with tag "stable".
		{
			name: "revision with stable tag",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-003", Tag: "candidate"},
				{RevisionName: "test-003", Percent: 50},
				{RevisionName: "test-001", Percent: 50, Tag: rollout.StableTag},
			},
			expected: "test-001",
		},
		// The same revision has stable tag and receive all the traffic.
		{
			name: "revision with stable tag and 100% traffic",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-003", Tag: "new"},
				{RevisionName: "test-002", Percent: 100, Tag: "stable"},
			},
			expected: "test-002",
		},
		// There's a revision with tag "stable" but another revision handles 100% traffic.
		{
			name: "revision with stable tag but another handles traffic",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 100},
				{RevisionName: "test-001", Percent: 0, Tag: rollout.StableTag},
			},
			expected: "test-002",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := &ServiceOpts{Traffic: test.traffic}
			svc := generateService(opts)
			stable := rollout.DetectStableRevisionName(svc)

			assert.Equal(t, test.expected, stable)
		})
	}
}

func TestDetectCandidateRevisionName(t *testing.T) {
	var tests = []struct {
		name           string
		annotations    map[string]string
		traffic        []*run.TrafficTarget
		latestReady    string
		stableRevision string
		expected       string
	}{
		// Latest revision is the same as the stable one.
		{
			name:           "same latest and stable revisions",
			latestReady:    "test-001",
			stableRevision: "test-001",
			expected:       "",
		},
		// Latest revision is not the same as the stable one.
		{
			name:           "different latest and stable revisions",
			latestReady:    "test-002",
			stableRevision: "test-001",
			expected:       "test-002",
		},
		// Latest revision is not the same as the stable one, but latest was unhealthy.
		{
			name: "latest revision was previously unhealthy",
			annotations: map[string]string{
				rollout.LastFailedCandidateRevisionAnnotation: "test-002",
			},
			latestReady:    "test-002",
			stableRevision: "test-001",
			expected:       "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := &ServiceOpts{
				Traffic:             test.traffic,
				Annotations:         test.annotations,
				LatestReadyRevision: test.latestReady,
			}
			svc := generateService(opts)
			candidate := rollout.DetectCandidateRevisionName(svc, test.stableRevision)

			assert.Equal(t, test.expected, candidate)
		})
	}
}
