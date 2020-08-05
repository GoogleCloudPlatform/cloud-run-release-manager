package rollout_test

import (
	"context"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	metricsMocker "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics/mock"
	runMocker "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/run/mock"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/rollout"
	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/run/v1"
)

type ServiceOpts struct {
	Name                  string
	Annotations           map[string]string
	LatestReadyRevision   string
	LatestCreatedRevision string
	Traffic               []*run.TrafficTarget
}

func generateService(opts *ServiceOpts) *run.Service {
	return &run.Service{
		Metadata: &run.ObjectMeta{
			Annotations: opts.Annotations,
		},
		Spec: &run.ServiceSpec{
			Traffic: opts.Traffic,
		},
		Status: &run.ServiceStatus{
			Traffic:                 opts.Traffic,
			LatestReadyRevisionName: opts.LatestReadyRevision,
		},
	}
}

func TestUpdateService(t *testing.T) {
	runclient := &runMocker.RunAPI{}
	metricsMock := &metricsMocker.Metrics{}
	metricsMock.RequestCountFn = func(ctx context.Context, offset time.Duration) (int64, error) {
		return 1000, nil
	}
	metricsMock.LatencyFn = func(ctx context.Context, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
		return 500, nil
	}
	metricsMock.ErrorRateFn = func(ctx context.Context, offset time.Duration) (float64, error) {
		return 0.01, nil
	}
	metricsMock.SetCandidateRevisionFn = func(revisionName string) {}
	strategy := &config.Strategy{
		Steps:              []int64{10, 40, 70},
		HealthOffsetMinute: 5,
	}

	var tests = []struct {
		name        string
		traffic     []*run.TrafficTarget
		annotations map[string]string
		lastReady   string

		// See the metrics mock to know what would make the diagnosis the needed
		// value for testing.
		healthCriteria []config.Metric
		outAnnotations map[string]string
		outTraffic     []*run.TrafficTarget
		shouldErr      bool
		nilService     bool
	}{
		// There is a revision with 100% of traffic different from stable and candidate.
		// Make it the stable revision.
		{
			name: "stable revision based on traffic share",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Tag: rollout.StableTag},
				{RevisionName: "test-002", Percent: 100},
				{RevisionName: "test-003", Percent: 0, Tag: rollout.CandidateTag},
			},
			lastReady: "test-003",
			healthCriteria: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Type: config.ErrorRateMetricsCheck, Threshold: 5},
			},
			outAnnotations: map[string]string{
				rollout.StableRevisionAnnotation:    "test-002",
				rollout.CandidateRevisionAnnotation: "test-003",
			},
			outTraffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 100 - strategy.Steps[0], Tag: rollout.StableTag},
				{RevisionName: "test-003", Percent: strategy.Steps[0], Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
		},
		// There's no a stable revision nor a revision handling 100% of traffic.
		{
			name: "no stable revision",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 50},
				{RevisionName: "test-001", Percent: 50},
			},
			lastReady:  "test-002",
			nilService: true,
		},
		// Stable revision is the same as the latest revision. There's no candidate.
		{
			name: "same stable and latest revision",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100},
			},
			lastReady:  "test-001",
			nilService: true,
		},
		// Candidate is new with non-existing previous candidate.
		{
			name: "new candidate and non-existing previous candidate",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100, Tag: rollout.StableTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			lastReady: "test-002",
			outAnnotations: map[string]string{
				rollout.StableRevisionAnnotation:    "test-001",
				rollout.CandidateRevisionAnnotation: "test-002",
			},
			outTraffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100 - strategy.Steps[0], Tag: rollout.StableTag},
				{RevisionName: "test-002", Percent: strategy.Steps[0], Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
		},
		// Candidate is the same as before (and healthy), keep rolling forward.
		{
			name: "keep rolling out the same candidate",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100 - strategy.Steps[1], Tag: rollout.StableTag},
				{RevisionName: "test-002", Percent: strategy.Steps[1], Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			lastReady: "test-002",
			healthCriteria: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Type: config.ErrorRateMetricsCheck, Threshold: 5},
			},
			outAnnotations: map[string]string{
				rollout.StableRevisionAnnotation:    "test-001",
				rollout.CandidateRevisionAnnotation: "test-002",
			},
			outTraffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100 - strategy.Steps[2], Tag: rollout.StableTag},
				{RevisionName: "test-002", Percent: strategy.Steps[2], Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
		},
		// Candidate is not the same as before, restart rollout with new candidate.
		{
			name: "different candidate, restart rollout",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100 - strategy.Steps[2], Tag: rollout.StableTag},
				{RevisionName: "test-002", Percent: strategy.Steps[2], Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			lastReady: "test-003",
			outAnnotations: map[string]string{
				rollout.StableRevisionAnnotation:    "test-001",
				rollout.CandidateRevisionAnnotation: "test-003",
			},
			outTraffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100 - strategy.Steps[0], Tag: rollout.StableTag},
				{RevisionName: "test-003", Percent: strategy.Steps[0], Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
		},
		// Candidate was handling 100% of traffic. It's now ready to become stable.
		{
			name: "candidate is ready to become stable",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 100, Tag: rollout.CandidateTag},
				{RevisionName: "test-001", Percent: 0, Tag: rollout.StableTag},
			},
			lastReady: "test-002",
			healthCriteria: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 750},
				{Type: config.ErrorRateMetricsCheck, Threshold: 5},
			},
			outAnnotations: map[string]string{
				rollout.StableRevisionAnnotation: "test-002",
			},
			outTraffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 100, Tag: rollout.StableTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
		},
		// Candidate is unhealthy, rollback.
		{
			name: "unhealthy candidate, rollback",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 20, Tag: rollout.CandidateTag},
				{RevisionName: "test-001", Percent: 80, Tag: rollout.StableTag},
			},
			lastReady: "test-002",
			healthCriteria: []config.Metric{
				{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: 100},
				{Type: config.ErrorRateMetricsCheck, Threshold: 0.95},
			},
			outAnnotations: map[string]string{
				rollout.StableRevisionAnnotation:              "test-001",
				rollout.CandidateRevisionAnnotation:           "test-002",
				rollout.LastFailedCandidateRevisionAnnotation: "test-002",
			},
			outTraffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100, Tag: rollout.StableTag},
				{RevisionName: "test-002", Percent: 0, Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
		},
		// Last ready revision is a previously failed candidate.
		{
			name: "latest ready is a failed candidate",
			annotations: map[string]string{
				rollout.LastFailedCandidateRevisionAnnotation: "test-002",
			},
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 100},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			lastReady:  "test-002",
			nilService: true,
		},
		// Inconclusive diagnosis, do nothing.
		{
			name: "inconclusive diagnosis",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 20, Tag: rollout.CandidateTag},
				{RevisionName: "test-001", Percent: 80, Tag: rollout.StableTag},
			},
			lastReady: "test-002",
			healthCriteria: []config.Metric{
				{Type: config.RequestCountMetricsCheck, Threshold: 1500},
				{Type: config.ErrorRateMetricsCheck, Threshold: 0.95},
			},
			nilService: true,
		},
		// Unknown diagnosis, should err.
		{
			name: "unknown diagnosis",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-002", Percent: 20, Tag: rollout.CandidateTag},
				{RevisionName: "test-001", Percent: 80, Tag: rollout.StableTag},
			},
			lastReady: "test-002",
			healthCriteria: []config.Metric{
				{Type: config.RequestCountMetricsCheck, Threshold: 500},
			},
			shouldErr: true,
		},
	}

	for _, test := range tests {
		runclient.ReplaceServiceFn = func(namespace, serviceID string, svc *run.Service) (*run.Service, error) {
			return svc, nil
		}

		opts := &ServiceOpts{
			Name:                "mysvc",
			Annotations:         test.annotations,
			LatestReadyRevision: test.lastReady,
			Traffic:             test.traffic,
		}
		svc := generateService(opts)
		svcRecord := &rollout.ServiceRecord{Service: svc}

		strategy.HealthCriteria = test.healthCriteria
		lg := logrus.New()
		lg.SetLevel(logrus.DebugLevel)
		r := rollout.New(context.TODO(), metricsMock, svcRecord, strategy).WithClient(runclient).WithLogger(lg)

		t.Run(test.name, func(t *testing.T) {
			svc, err := r.UpdateService(svc)
			if test.shouldErr {
				assert.NotNil(t, err)
			} else if test.nilService {
				assert.Nil(t, svc)
			} else {
				assert.True(t, cmp.Equal(test.outAnnotations, svc.Metadata.Annotations))
				assert.True(t, cmp.Equal(test.outTraffic, svc.Spec.Traffic))
			}
		})

	}
}

func TestPrepareRollForward(t *testing.T) {
	runclient := &runMocker.RunAPI{}
	metricsMock := &metricsMocker.Metrics{}
	strategy := &config.Strategy{
		Steps: []int64{5, 30, 60},
	}

	var tests = []struct {
		name      string
		stable    string
		candidate string
		traffic   []*run.TrafficTarget
		expected  []*run.TrafficTarget
	}{
		// There's a new candidate. Restart rollout process
		{
			name:      "new candidate, restart rollout",
			stable:    "test-001",
			candidate: "test-003",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 50},
				{RevisionName: "test-001", Tag: "tag1"},
				{RevisionName: "test-002", Percent: 50, Tag: rollout.CandidateTag},
				{RevisionName: "test-002", Tag: "tag2"},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			expected: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 95, Tag: rollout.StableTag},
				{RevisionName: "test-003", Percent: 5, Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
				{RevisionName: "test-001", Tag: "tag1"},
				{RevisionName: "test-002", Tag: "tag2"},
			},
		},
		// Candidate is the same. Continue rolling forward.
		{
			name:      "continue rolling out candidate",
			stable:    "test-001",
			candidate: "test-003",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 70, Tag: rollout.StableTag},
				{RevisionName: "test-002", Tag: "tag1"},
				{RevisionName: "test-003", Percent: 30, Tag: rollout.CandidateTag},
				{RevisionName: "test-003", Tag: "tag2"},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			expected: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 40, Tag: rollout.StableTag},
				{RevisionName: "test-003", Percent: 60, Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
				{RevisionName: "test-002", Tag: "tag1"},
				{RevisionName: "test-003", Tag: "tag2"},
			},
		},
		// Candidate is the same. Continue rolling forward to 100%.
		{
			name:      "roll out to 100%",
			stable:    "test-001",
			candidate: "test-003",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 40, Tag: rollout.StableTag},
				{RevisionName: "test-002", Tag: "tag1"},
				{RevisionName: "test-003", Percent: 60, Tag: rollout.CandidateTag},
				{RevisionName: "test-003", Tag: "tag2"},
			},
			expected: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 0, Tag: rollout.StableTag},
				{RevisionName: "test-003", Percent: 100, Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
				{RevisionName: "test-002", Tag: "tag1"},
				{RevisionName: "test-003", Tag: "tag2"},
			},
		},
		// Candidate has proven able to handle 100%, make it stable.
		{
			name:      "make candidate stable",
			stable:    "test-001",
			candidate: "test-003",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 0, Tag: rollout.StableTag},
				{RevisionName: "test-002", Tag: "tag1"},
				{RevisionName: "test-003", Percent: 100, Tag: rollout.CandidateTag},
				{RevisionName: "test-003", Tag: "tag2"},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			expected: []*run.TrafficTarget{
				{RevisionName: "test-003", Percent: 100, Tag: rollout.StableTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
				{RevisionName: "test-002", Tag: "tag1"},
				{RevisionName: "test-003", Tag: "tag2"},
			},
		},
		// Two targets for the same stable and candidate revisions.
		{
			name:      "two targets for same revision",
			stable:    "test-001",
			candidate: "test-003",
			traffic: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 70},
				{RevisionName: "test-001", Tag: rollout.StableTag},
				{RevisionName: "test-002", Tag: "tag1"},
				{RevisionName: "test-003", Percent: 30},
				{RevisionName: "test-003", Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
			},
			expected: []*run.TrafficTarget{
				{RevisionName: "test-001", Percent: 40, Tag: rollout.StableTag},
				{RevisionName: "test-003", Percent: 60, Tag: rollout.CandidateTag},
				{LatestRevision: true, Tag: rollout.LatestTag},
				{RevisionName: "test-002", Tag: "tag1"},
			},
		},
	}

	for _, test := range tests {
		opts := &ServiceOpts{
			Traffic: test.traffic,
		}
		svc := generateService(opts)
		svcRecord := &rollout.ServiceRecord{Service: svc}

		r := rollout.New(context.TODO(), metricsMock, svcRecord, strategy).WithClient(runclient)

		t.Run(test.name, func(t *testing.T) {
			svc = r.PrepareRollForward(svc, test.stable, test.candidate)
			assert.True(t, cmp.Equal(test.expected, svc.Spec.Traffic))
		})
	}
}

func TestPrepareRollback(t *testing.T) {
	metricsMock := &metricsMocker.Metrics{}

	stable := "test-001"
	candidate := "test-003"
	traffic := []*run.TrafficTarget{
		{RevisionName: "test-001", Percent: 40, Tag: rollout.StableTag},
		{RevisionName: "test-002", Tag: "tag1"},
		{RevisionName: "test-003", Percent: 60, Tag: rollout.CandidateTag},
		{RevisionName: "test-003", Tag: "tag2"},
	}
	expectedTraffic := []*run.TrafficTarget{
		{RevisionName: "test-001", Percent: 100, Tag: rollout.StableTag},
		{RevisionName: "test-003", Percent: 0, Tag: rollout.CandidateTag},
		{LatestRevision: true, Tag: rollout.LatestTag},
		{RevisionName: "test-002", Tag: "tag1"},
		{RevisionName: "test-003", Tag: "tag2"},
	}
	svc := generateService(&ServiceOpts{Traffic: traffic})
	svcRecord := &rollout.ServiceRecord{Service: svc}

	r := rollout.New(context.TODO(), metricsMock, svcRecord, &config.Strategy{})
	svc = r.PrepareRollback(svc, stable, candidate)
	assert.Equal(t, expectedTraffic, svc.Spec.Traffic)
}
