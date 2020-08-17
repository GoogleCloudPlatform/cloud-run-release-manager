package stackdriver

import (
	"context"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/metrics"
	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	// TODO: Migrate to cloud.google.com/go/monitoring/apiv3/v2 once RPC for MQL
	// query is added (https://cloud.google.com/monitoring/api/ref_v3/rest/v3/projects.timeSeries/query).
	monitoring "google.golang.org/api/monitoring/v3"
)

// query is the filter used to retrieve metrics data.
type query string

// Provider is a metrics provider for Cloud Monitoring.
type Provider struct {
	metricsClient *monitoring.Service
	project       string

	// query is used to filter the metrics for the wanted resource.
	query
}

// Metric types.
const (
	requestLatencies = "run.googleapis.com/request_latencies"
	requestCount     = "run.googleapis.com/request_count"
)

// NewProvider initializes the provider for Cloud Monitoring.
func NewProvider(ctx context.Context, project string, region string, serviceName string) (*Provider, error) {
	client, err := monitoring.NewService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize Cloud Metics client")
	}

	return &Provider{
		metricsClient: client,
		project:       project,
		query:         newQuery(project, region, serviceName),
	}, nil
}

// SetCandidateRevision sets the candidate revision name for which the provider
// should get metrics.
func (p *Provider) SetCandidateRevision(revisionName string) {
	p.query = p.query.addFilter("resource.labels.revision_name", revisionName)
}

// RequestCount count returns the number of requests for the given offset.
func (p *Provider) RequestCount(ctx context.Context, offset time.Duration) (int64, error) {
	query := p.addFilter("metric.type", requestCount)
	endTime := time.Now()
	endTimeString := endTime.Format(time.RFC3339Nano)
	startTime := endTime.Add(-1 * offset)
	startTimeString := startTime.Format(time.RFC3339Nano)
	offsetString := fmt.Sprintf("%fs", offset.Seconds())

	req := p.metricsClient.Projects.TimeSeries.List("projects/" + p.project).
		Filter(string(query)).
		IntervalStartTime(startTimeString).
		IntervalEndTime(endTimeString).
		AggregationAlignmentPeriod(offsetString).
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationGroupByFields("resource.labels.service_name").
		AggregationCrossSeriesReducer("REDUCE_SUM")

	logger := util.LoggerFrom(ctx).WithFields(logrus.Fields{
		"intervalStartTime": startTimeString,
		"intervalEndTime":   endTimeString,
		"metrics":           "request-count",
	})
	logger.Debug("querying Cloud Monitoring API")
	timeSeries, err := makeRequestForTimeSeries(logger, req)
	if err != nil {
		return 0, errors.Wrap(err, "error when querying for time series")
	}

	// This happens when no request was made during the given offset.
	if len(timeSeries) == 0 {
		return 0, nil
	}
	// The request count is aggregated for the entire service, so only one time
	// series and a point is returned. There's no need for a loop.
	series := timeSeries[0]
	if len(series.Points) == 0 {
		return 0, errors.New("no data point was retrieved")
	}
	return *(series.Points[0].Value.Int64Value), nil
}

// Latency returns the latency for the resource for the given offset.
// It returns 0 if no request was made during the interval.
func (p *Provider) Latency(ctx context.Context, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
	query := p.query.addFilter("metric.type", requestLatencies)
	endTime := time.Now()
	endTimeString := endTime.Format(time.RFC3339Nano)
	startTime := endTime.Add(-1 * offset)
	startTimeString := startTime.Format(time.RFC3339Nano)
	aligner, reducer := alignerAndReducer(alignReduceType)
	offsetString := fmt.Sprintf("%fs", offset.Seconds())

	req := p.metricsClient.Projects.TimeSeries.List("projects/" + p.project).
		Filter(string(query)).
		IntervalStartTime(startTimeString).
		IntervalEndTime(endTimeString).
		AggregationAlignmentPeriod(offsetString).
		AggregationPerSeriesAligner(aligner).
		AggregationGroupByFields("resource.labels.service_name").
		AggregationCrossSeriesReducer(reducer)

	logger := util.LoggerFrom(ctx).WithFields(logrus.Fields{
		"intervalStartTime": startTimeString,
		"intervalEndTime":   endTimeString,
		"metrics":           "latency",
		"aligner":           aligner,
		"reducer":           reducer,
	})
	logger.Debug("querying Cloud Monitoring API")
	timeSeries, err := makeRequestForTimeSeries(logger, req)
	if err != nil {
		return 0, errors.Wrap(err, "error when querying for time series")
	}

	// This happens when no request was made during the given offset.
	if len(timeSeries) == 0 {
		return 0, nil
	}
	// The request count is aggregated for the entire service, so only one time
	// series and a point is returned. There's no need for a loop.
	series := timeSeries[0]
	if len(series.Points) == 0 {
		return 0, errors.New("no data point was retrieved")
	}
	return *(series.Points[0].Value.DoubleValue), nil
}

// ErrorRate returns the rate of 5xx errors for the resource in the given offset.
// It returns 0 if no request was made during the interval.
func (p *Provider) ErrorRate(ctx context.Context, offset time.Duration) (float64, error) {
	query := p.query.addFilter("metric.type", requestCount)
	endTime := time.Now()
	endTimeString := endTime.Format(time.RFC3339Nano)
	startTime := endTime.Add(-1 * offset)
	startTimeString := startTime.Format(time.RFC3339Nano)
	offsetString := fmt.Sprintf("%fs", offset.Seconds())

	req := p.metricsClient.Projects.TimeSeries.List("projects/" + p.project).
		Filter(string(query)).
		IntervalStartTime(startTimeString).
		IntervalEndTime(endTimeString).
		AggregationAlignmentPeriod(offsetString).
		AggregationPerSeriesAligner("ALIGN_DELTA").
		AggregationGroupByFields("metric.labels.response_code_class").
		AggregationCrossSeriesReducer("REDUCE_SUM")

	logger := util.LoggerFrom(ctx).WithFields(logrus.Fields{
		"intervalStartTime": startTimeString,
		"intervalEndTime":   endTimeString,
		"metrics":           "error-rate",
	})
	logger.Debug("querying Cloud Monitoring API")
	timeSeries, err := makeRequestForTimeSeries(logger, req)
	if err != nil {
		return 0, errors.Wrap(err, "error when querying for time series")
	}

	// This happens when no request was made during the given offset.
	if len(timeSeries) == 0 {
		return 0, nil
	}
	return calculateErrorResponseRate(timeSeries)
}

func makeRequestForTimeSeries(logger *logrus.Entry, req *monitoring.ProjectsTimeSeriesListCall) ([]*monitoring.TimeSeries, error) {
	resp, err := req.Do()
	if err != nil {
		return nil, errors.Wrap(err, "error when retrieving time series")
	}
	if len(resp.ExecutionErrors) != 0 {
		for _, execError := range resp.ExecutionErrors {
			logger.WithField("message", execError.Message).Warn("execution error occurred")
		}
		return nil, errors.Errorf("execution errors occurred")
	}

	return resp.TimeSeries, nil
}

// calculateErrorResponseRate calculates the percentage of 5xx error response.
//
// It gets all the server responses and calculates the error rate by performing
// the operation (5xx responses / all responses). Then, it divides the number of
// error responses by the total.
func calculateErrorResponseRate(timeSeries []*monitoring.TimeSeries) (float64, error) {
	var errorResponseCount, totalResponses int64
	for _, series := range timeSeries {
		// Because the interval and the series aligner are the same, only one
		// point is returned per time series.
		switch series.Metric.Labels["response_code_class"] {
		case "5xx":
			errorResponseCount += *(series.Points[0].Value.Int64Value)
		default:
			totalResponses += *(series.Points[0].Value.Int64Value)
		}
	}

	totalResponses += errorResponseCount
	if totalResponses == 0 {
		return 0, nil
	}

	rate := float64(errorResponseCount) / float64(totalResponses)
	return rate, nil
}

func alignerAndReducer(alignReduceType metrics.AlignReduce) (aligner string, reducer string) {
	switch alignReduceType {
	case metrics.Align99Reduce99:
		aligner = "ALIGN_PERCENTILE_99"
		reducer = "REDUCE_PERCENTILE_99"
	case metrics.Align95Reduce95:
		aligner = "ALIGN_PERCENTILE_95"
		reducer = "REDUCE_PERCENTILE_50"
	case metrics.Align50Reduce50:
		aligner = "ALIGN_PERCENTILE_50"
		reducer = "REDUCE_PERCENTILE_50"
	}

	return
}

// newQuery initializes a query.
func newQuery(project, region, serviceName string) query {
	var q query
	return q.addFilter("resource.labels.project_id", project).
		addFilter("resource.labels.location", region).
		addFilter("resource.labels.service_name", serviceName)
}

// addFilter adds a filter to the query.
//
// TODO: Support field-based filters, so the query string is generated based on
// the fields instead of appending a filter everytime this method is called.
func (q query) addFilter(key, value string) query {
	if q != "" {
		q += " AND "
	}
	q += query(fmt.Sprintf("%s=%q", key, value))

	return q
}
