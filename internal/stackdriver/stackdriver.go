package stackdriver

import (
	"context"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	"github.com/pkg/errors"

	// TODO: Migrate to cloud.google.com/go/monitoring/apiv3/v2 once RPC for MQL
	// query is added (https://cloud.google.com/monitoring/api/ref_v3/rest/v3/projects.timeSeries/query).
	monitoring "google.golang.org/api/monitoring/v3"
)

// API is a wrapper for the Cloud Monitoring package.
type API struct {
	MetricClient *monitoring.Service
	Project      string
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
	client, err := monitoring.NewService(ctx)
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
	startTime := endTime.Add(-1 * offset)
	aligner, reducer := alignerAndReducer(alignReduceType)
	offsetString := fmt.Sprintf("%fs", offset.Seconds())

	req := a.MetricClient.Projects.TimeSeries.List("projects/" + a.Project).
		Filter(query.Query()).
		IntervalStartTime(startTime.Format(time.RFC3339Nano)).
		IntervalEndTime(endTime.Format(time.RFC3339Nano)).
		AggregationAlignmentPeriod(offsetString).
		AggregationPerSeriesAligner(aligner).
		AggregationGroupByFields("metric.labels.response_code_class").
		AggregationCrossSeriesReducer(reducer)

	resp, err := req.Do()
	if err != nil {
		return 0, errors.Wrap(err, "error when retrieving time series")
	}
	if len(resp.ExecutionErrors) != 0 {
		return 0, errors.Errorf("execution errors occurred: %v", resp.ExecutionErrors)
	}

	return latencyForCodeClass(resp.TimeSeries, "2xx")
}

// ErrorRate returns the rate of 5xx errors for the resource matching the filter.
func (a *API) ErrorRate(ctx context.Context, query metrics.Query, offset time.Duration) (float64, error) {
	query = addFilterToQuery(query, "metric.type", requestCount)
	endTime := time.Now()
	startTime := endTime.Add(-1 * offset)
	offsetString := fmt.Sprintf("%fs", offset.Seconds())

	req := a.MetricClient.Projects.TimeSeries.List("projects/" + a.Project).
		Filter(query.Query()).
		IntervalStartTime(startTime.Format(time.RFC3339Nano)).
		IntervalEndTime(endTime.Format(time.RFC3339Nano)).
		AggregationAlignmentPeriod(offsetString).
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationGroupByFields("metric.labels.response_code_class").
		AggregationCrossSeriesReducer("REDUCE_SUM")

	resp, err := req.Do()
	if err != nil {
		return 0, errors.Wrap(err, "error when retrieving time series")
	}
	if len(resp.ExecutionErrors) != 0 {
		return 0, errors.Errorf("execution errors occurred: %v", resp.ExecutionErrors)
	}

	return calculateErrorResponseRate(resp.TimeSeries)
}

// latencyForCodeClass retrieves the latency for a given response code class
// (e.g. 2xx, 5xx, etc.)
func latencyForCodeClass(timeSeries []*monitoring.TimeSeries, codeClass string) (float64, error) {
	var latency float64
	for _, series := range timeSeries {
		// Because the interval and the series aligner are the same, only one
		// point is returned per time series.
		if series.Metric.Labels["response_code_class"] == codeClass {
			latency = *(series.Points[0].Value.DoubleValue)
			break
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
func calculateErrorResponseRate(timeSeries []*monitoring.TimeSeries) (float64, error) {
	var errorResponseCount, successfulResponseCount int64
	for _, series := range timeSeries {
		// Because the interval and the series aligner are the same, only one
		// point is returned per time series.
		switch series.Metric.Labels["response_code_class"] {
		case "5xx":
			errorResponseCount += *(series.Points[0].Value.Int64Value)
			break
		default:
			successfulResponseCount += *(series.Points[0].Value.Int64Value)
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

func alignerAndReducer(alignReduceType metrics.AlignReduce) (aligner string, reducer string) {
	switch alignReduceType {
	case metrics.Align99Reduce99:
		aligner = "ALIGN_PERCENTILE_99"
		reducer = "REDUCE_PERCENTILE_99"
		break
	case metrics.Align95Reduce95:
		aligner = "ALIGN_PERCENTILE_95"
		reducer = "REDUCE_PERCENTILE_50"
		break
	case metrics.Align50Reduce50:
		aligner = "ALIGN_PERCENTILE_50"
		reducer = "REDUCE_PERCENTILE_50"
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
