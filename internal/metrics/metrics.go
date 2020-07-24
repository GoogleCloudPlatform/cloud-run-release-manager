package metrics

import (
	"context"
	"time"

	"github.com/pkg/errors"
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

// PercentileToAlignReduce takes a percentile value maps it to a AlignReduce
// value.
//
// TODO: once we start supporting any percentile value, this should not be
// needed.
func PercentileToAlignReduce(percentile float64) (AlignReduce, error) {
	switch percentile {
	case 99:
		return Align99Reduce99, nil
	case 95:
		return Align95Reduce95, nil
	case 50:
		return Align50Reduce50, nil
	default:
		return 0, errors.Errorf("unsupported percentile value %.2f", percentile)
	}
}
