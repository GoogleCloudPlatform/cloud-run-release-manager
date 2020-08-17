// Package sheets provides a metrics provider implementation that retrieves
// metrics from a publicly-available Google Sheets
//
// The document must have the following values, starting at row 2, in the
// following order.
//
// Region, Service, Request Count, Error Rate, Latency P99, Latency P95, Latency P50
//
// Example
// us-east1, tester, 1000, 0.01, 1000, 750, 500
package sheets

import (
	"context"
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/metrics"
	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/sheets/v4"
)

// Column numbers
const (
	colRegion = iota
	colServiceName
	colRequestCount
	colErrorRate
	colLatencyP99
	colLatencyP95
	colLatencyP50
)

// Provider is a metrics provider for Google Sheets.
type Provider struct {
	client      *sheets.Service
	sheetsID    string
	sheetName   string
	region      string
	serviceName string
}

// NewProvider initializes a connection to Google Sheets
func NewProvider(ctx context.Context, sheetsID, sheetName, region, serviceName string) (*Provider, error) {
	client, err := sheets.NewService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize Google Sheets client")
	}
	if sheetsID == "" {
		return nil, errors.New("Google Sheet ID cannot be empty")
	}

	return &Provider{
		client:      client,
		sheetsID:    sheetsID,
		sheetName:   sheetName,
		region:      region,
		serviceName: serviceName,
	}, nil
}

// SetCandidateRevision sets the candidate revision name for which the provider
// should get metrics.
//
// For Google Sheets, ignore this since the data in the document is always for
// the candidate revision.
func (p *Provider) SetCandidateRevision(revisionName string) {}

// RequestCount returns the number of requests for the given offset.
func (p *Provider) RequestCount(ctx context.Context, offset time.Duration) (int64, error) {
	logger := util.LoggerFrom(ctx)
	logger.Debug("querying google sheet for request count")
	serviceRow, err := p.retrieveServiceRow(logger)
	if err != nil {
		return 0, errors.Wrap(err, "failed to retrieve metrics for the service")
	}

	col := serviceRow[colRequestCount]
	requestCount, ok := col.(string)
	if !ok {
		return 0, errors.Errorf("invalid request count value, must be a string but has value %v of type %T", col, col)
	}
	value, err := strconv.ParseInt(requestCount, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse request count")
	}
	return value, nil
}

// Latency returns the latency for the resource for the given offset.
func (p *Provider) Latency(ctx context.Context, offset time.Duration, alignReduceType metrics.AlignReduce) (float64, error) {
	logger := util.LoggerFrom(ctx)
	logger.Debug("querying google sheet for request count")
	serviceRow, err := p.retrieveServiceRow(logger)
	if err != nil {
		return 0, errors.Wrap(err, "failed to retrieve metrics for the service")
	}

	var col interface{}
	switch alignReduceType {
	case metrics.Align99Reduce99:
		col = serviceRow[colLatencyP99]
		break
	case metrics.Align95Reduce95:
		col = serviceRow[colLatencyP95]
		break
	case metrics.Align50Reduce50:
		col = serviceRow[colLatencyP50]
		break
	}

	latency, ok := col.(string)
	if !ok {
		return 0, errors.Errorf("invalid latency value, must be a string but has value %v of type %T", col, col)
	}
	value, err := strconv.ParseFloat(latency, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse latency")
	}
	return value, nil
}

// ErrorRate returns the rate of 5xx errors for the resource matching the filter.
func (p *Provider) ErrorRate(ctx context.Context, offset time.Duration) (float64, error) {
	logger := util.LoggerFrom(ctx)
	logger.Debug("querying google sheet for request count")
	serviceRow, err := p.retrieveServiceRow(logger)
	if err != nil {
		return 0, errors.Wrap(err, "failed to retrieve metrics for the service")
	}

	col := serviceRow[colErrorRate]
	errorRate, ok := col.(string)
	if !ok {
		return 0, errors.Errorf("invalid error rate value, must be a string but has value %v of type %T", col, col)
	}
	value, err := strconv.ParseFloat(errorRate, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse error rate")
	}
	return value, nil
}

// retrieveServiceRow returns the row that contains the information about the
// service
func (p *Provider) retrieveServiceRow(logger *logrus.Entry) ([]interface{}, error) {
	values, err := p.retrieveValues(logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve values")
	}

	serviceRow, err := p.filterServiceRow(values)
	if err != nil {
		return nil, errors.Wrap(err, "failed to filter service row")
	}
	if serviceRow == nil {
		return nil, errors.Errorf("no service matched the query, region=%q service=%q", p.region, p.serviceName)
	}
	return serviceRow, nil
}

// retrieveValues get all the metrics values starting at row 2.
func (p *Provider) retrieveValues(logger *logrus.Entry) ([][]interface{}, error) {
	readRange := "A2:G"
	if p.sheetName != "" {
		readRange = p.sheetName + "!" + readRange
	}
	resp, err := p.client.Spreadsheets.Values.Get(p.sheetsID, readRange).Do()
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve data from sheet")
	}
	logger.Debugf("queried %d rows", len(resp.Values))

	return resp.Values, nil
}

// filterServiceRow returns the first row that matches the region and service
// name.
func (p *Provider) filterServiceRow(values [][]interface{}) ([]interface{}, error) {
	for _, row := range values {
		col := row[colRegion]
		region, ok := col.(string)
		if !ok {
			return nil, errors.Errorf("invalid region value, must be a string but has value %v of type %T", col, col)
		}
		col = row[colServiceName]
		serviceName, ok := col.(string)
		if !ok {
			return nil, errors.Errorf("invalid service name value, must be a string but has value %v of type %T", col, col)
		}
		if region == p.region && serviceName == p.serviceName {
			return row, nil
		}
	}
	return nil, nil
}
