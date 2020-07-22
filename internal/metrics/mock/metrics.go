package mock

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
)

// Metrics is a mock implementation of metrics.Metrics.
type Metrics struct {
	LatencyFn      func(ctx context.Context, query metrics.Query, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error)
	LatencyInvoked bool

	ErrorRateFn      func(ctx context.Context, query metrics.Query, offset time.Duration) (float64, error)
	ErrorRateInvoked bool
}

// Query is a mock implementation of metrics.Query.
type Query struct{}

// Latency invokes the mock implementation and marks the function as invoked.
func (m *Metrics) Latency(ctx context.Context, query metrics.Query, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
	m.LatencyInvoked = true
	return m.LatencyFn(ctx, query, offset, alignReduceType)
}

// ErrorRate invokes the mock implementation and marks the function as invoked.
func (m *Metrics) ErrorRate(ctx context.Context, query metrics.Query, offset time.Duration) (float64, error) {
	m.ErrorRateInvoked = true
	return m.ErrorRateFn(ctx, query, offset)
}

// Query returns an empty string to comply with the interface.
func (q Query) Query() string {
	return ""
}
