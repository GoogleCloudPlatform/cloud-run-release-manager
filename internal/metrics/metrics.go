package metrics

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// AlignReduce is the type to enumerate allowed combinations of per series
// aligner and cross series reducer.
type AlignReduce int32

// Series aligner and cross series reducer types (for latency).
const (
	Unknown AlignReduce = iota
	Align99Reduce99
	Align95Reduce95
	Align50Reduce50
)

// Provider represents a metrics Provider such as Stackdriver.
type Provider interface {
	// Sets the candidate revision name for which the provider should get
	// metrics.
	// TODO: Consider removing this method and making revisionName part of other
	// method signatures.
	SetCandidateRevision(revisionName string)

	// Returns the number of requests for the given offset and query.
	RequestCount(ctx context.Context, offset time.Duration) (int64, error)

	// Returns the request latency after applying the specified series aligner
	// and cross series reducer. The result is in milliseconds.
	// It returns 0 if no request was made during the interval.
	Latency(ctx context.Context, offset time.Duration, alignReduceType AlignReduce) (float64, error)

	// Gets all the server responses and calculates the error rate by performing
	// the operation (5xx responses / all responses).
	// It returns 0 if no request was made during the interval.
	ErrorRate(ctx context.Context, offset time.Duration) (float64, error)
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
