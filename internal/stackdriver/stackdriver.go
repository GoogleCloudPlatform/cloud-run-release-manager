package stackdriver

import (
	"context"
	"fmt"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	duration "google.golang.org/protobuf/types/known/durationpb"
	timestamp "google.golang.org/protobuf/types/known/timestamppb"
)

// API is a wrapper for the Cloud Monitoring package.
type API struct {
	*monitoring.MetricClient
	Project string
}

// Query is the filter used to retrieve metrics data.
type Query struct {
	filter string
}

// Metric types.
const (
	requestLatencies = "run.googleapis.com/request_latencies"
	requestCount     = "run.googleapis.com/request_count"
)

// NewAPIClient initializes
func NewAPIClient(ctx context.Context, project string) (*API, error) {
	client, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize Cloud Metics client")
	}

	return &API{
		MetricClient: client,
		Project:      project,
	}, nil
}

// Latency returns the latency for the resource matching the filter.
func (a *API) Latency(ctx context.Context, query metrics.Query, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
	query = addFilterToQuery(query, "metric.type", requestLatencies)
	endTime := time.Now()
	startTime := time.Now().Add(-1 * offset)
	aligner, reducer := alignerAndReducer(alignReduceType)

	it := a.MetricClient.ListTimeSeries(ctx, &monitoringpb.ListTimeSeriesRequest{
		Name:   "projects/" + a.Project,
		Filter: query.Query(),
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamp.New(startTime),
			EndTime:   timestamp.New(endTime),
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    duration.New(offset),
			PerSeriesAligner:   aligner,
			GroupByFields:      []string{"metric.labels.response_code_class"},
			CrossSeriesReducer: reducer,
		},
	})

	return latencyForCodeClass(it, "2xx")
}

// ErrorRate returns the rate of 5xx errors for the resource matching the filter.
func (a *API) ErrorRate(ctx context.Context, query metrics.Query, offset time.Duration) (float64, error) {
	query = addFilterToQuery(query, "metric.type", requestCount)
	endTime := time.Now()
	startTime := time.Now().Add(-1 * offset)

	it := a.MetricClient.ListTimeSeries(ctx, &monitoringpb.ListTimeSeriesRequest{
		Name:   "projects/" + a.Project,
		Filter: query.Query(),
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamp.New(startTime),
			EndTime:   timestamp.New(endTime),
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    duration.New(offset),
			PerSeriesAligner:   monitoringpb.Aggregation_ALIGN_DELTA,
			GroupByFields:      []string{"metric.labels.response_code_class"},
			CrossSeriesReducer: monitoringpb.Aggregation_REDUCE_SUM,
		},
	})

	return calculateErrorResponseRate(it)
}

// latencyForCodeClass retrieves the latency for a given response code class
// (e.g. 2xx, 5xx, etc.)
func latencyForCodeClass(it *monitoring.TimeSeriesIterator, codeClass string) (float64, error) {
	var latency float64
	for {
		series, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, errors.Wrap(err, "error when iterating through time series")
		}

		// Because the interval and the series aligner are the same, only one
		// point is returned per time series.
		if series.Metric.Labels["response_code_class"] == codeClass {
			latency = series.Points[0].Value.GetDoubleValue()
		}
	}

	return latency, nil
}

// calculateErrorResponseRate calculates the percentage of 5xx error response.
//
// It obtains all the successful responses (2xx) and the error responses (5xx),
// add them up to form a 'total'. Then, it divides the number of error responses
// by the total.
// It ignores any other type of responses (e.g. 4xx).
func calculateErrorResponseRate(it *monitoring.TimeSeriesIterator) (float64, error) {
	var errorResponseCount, successfulResponseCount int64
	for {
		series, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, errors.Wrap(err, "error when iterating through time series")
		}

		// Because the interval and the series aligner are the same, only one
		// point is returned per time series.
		switch series.Metric.Labels["response_code_class"] {
		case "5xx":
			errorResponseCount += series.Points[0].Value.GetInt64Value()
			break
		default:
			successfulResponseCount += series.Points[0].Value.GetInt64Value()
			break
		}
	}

	totalResponses := errorResponseCount + successfulResponseCount
	if totalResponses == 0 {
		return 0, errors.New("no requests in interval")
	}

	rate := float64(errorResponseCount) / float64(totalResponses)

	return rate, nil
}

func alignerAndReducer(alignReduceType metrics.AlignReduce) (aligner monitoringpb.Aggregation_Aligner, reducer monitoringpb.Aggregation_Reducer) {
	switch alignReduceType {
	case metrics.Align99Reduce99:
		aligner = monitoringpb.Aggregation_ALIGN_PERCENTILE_99
		reducer = monitoringpb.Aggregation_REDUCE_PERCENTILE_99
		break
	case metrics.Align95Reduce95:
		aligner = monitoringpb.Aggregation_ALIGN_PERCENTILE_95
		reducer = monitoringpb.Aggregation_REDUCE_PERCENTILE_95
		break
	case metrics.Align50Reduce50:
		aligner = monitoringpb.Aggregation_ALIGN_PERCENTILE_50
		reducer = monitoringpb.Aggregation_REDUCE_PERCENTILE_50
		break
	}

	return
}

// NewQuery initializes a query.
func NewQuery(project, region, serviceName, revisionName string) Query {
	return Query{}.addFilter("resource.labels.project_id", project).
		addFilter("resource.labels.location", region).
		addFilter("resource.labels.service_name", serviceName).
		addFilter("resource.labels.revision_name", revisionName)
}

// Query returns the string representation of the query.
func (q Query) Query() string {
	return q.filter
}

// addFilter adds a filter to the query.
func (q Query) addFilter(key, value string) Query {
	if q.filter != "" {
		q.filter += " AND "
	}
	q.filter += fmt.Sprintf("%s=%q", key, value)

	return q
}

func addFilterToQuery(query metrics.Query, key, value string) Query {
	q := query.(Query)

	return q.addFilter(key, value)
}
