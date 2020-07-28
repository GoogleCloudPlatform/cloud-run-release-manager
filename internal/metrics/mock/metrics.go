package mock

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
)

// Metrics is a mock implementation of metrics.Metrics.
type Metrics struct {
	SetCandidateRevisionFn      func(revisionName string)
	SetCandidateRevisionInvoked bool

	RequestCountFn      func(ctx context.Context, offset time.Duration) (int64, error)
	RequestCountInvoked bool

	LatencyFn      func(ctx context.Context, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error)
	LatencyInvoked bool

	ErrorRateFn      func(ctx context.Context, offset time.Duration) (float64, error)
	ErrorRateInvoked bool
}

// Query is a mock implementation of metrics.Query.
type Query struct{}

// SetCandidateRevision invokes the mock implementation and marks the function
// as invoked.
func (m *Metrics) SetCandidateRevision(revisionName string) {
	m.SetCandidateRevisionInvoked = true
	m.SetCandidateRevisionFn(revisionName)
}

// RequestCount invokes the mock implementation and marks the function as
// invoked.
func (m *Metrics) RequestCount(ctx context.Context, offset time.Duration) (int64, error) {
	m.RequestCountInvoked = true
	return m.RequestCountFn(ctx, offset)
}

// Latency invokes the mock implementation and marks the function as invoked.
func (m *Metrics) Latency(ctx context.Context, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
	m.LatencyInvoked = true
	return m.LatencyFn(ctx, offset, alignReduceType)
}

// ErrorRate invokes the mock implementation and marks the function as invoked.
func (m *Metrics) ErrorRate(ctx context.Context, offset time.Duration) (float64, error) {
	m.ErrorRateInvoked = true
	return m.ErrorRateFn(ctx, offset)
}

// Query returns an empty string to comply with the interface.
func (q Query) Query() string {
	return ""
}
