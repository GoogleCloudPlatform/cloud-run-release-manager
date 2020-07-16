package metrics

import (
	"context"
	"time"
)

// Query is the filter used to retrieve metrics data.
type Query interface {
	Query() string
}

// AlignReduce is the type to enumerate allowed combinations of per series
// aligner and cross series reducer.
type AlignReduce int32

// Series aligner and cross series reducer types (for latency).
const (
	Align99Reduce99 AlignReduce = 1
	Align95Reduce95             = 2
	Align50Reduce50             = 3
)

// Metrics represents a monitoring API such as Stackdriver.
//
// Latency returns the request latency after applying the specified series
// aligner and cross series reducer. The result is in milliseconds.
//
// Error rate gets all the server responses. It calculates the error rate by
// performing the operation (5xx responses / all responses).
type Metrics interface {
	Latency(ctx context.Context, query Query, offset time.Duration, alignReduceType AlignReduce) (float64, error)
	ErrorRate(ctx context.Context, query Query, offset time.Duration) (float64, error)
}
