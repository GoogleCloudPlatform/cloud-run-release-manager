// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	runapi "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/run"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/stackdriver"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/rollout"
	stackdriverFormatter "github.com/TV4/logrus-stackdriver-formatter"
	isatty "github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type stepFlags []int64

func (steps *stepFlags) Set(step string) error {
	value, err := strconv.ParseInt(step, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse step")
	}
	*steps = append(*steps, value)
	return nil
}

func (steps stepFlags) String() string {
	var value string
	for _, step := range steps {
		value += " " + strconv.FormatInt(step, 10)
	}

	return value
}

var (
	flLoggingLevel  string
	flCLI           bool
	flHTTPAddr      string
	flProject       string
	flLabelSelector string

	// Either service or label selection needed.
	flService string
	flLabel   string

	// Empty array means all regions.
	flRegions       []string
	flRegionsString string

	// Rollout strategy-related flags.
	flSteps       stepFlags
	flStepsString string
	flInterval    int64
	flErrorRate   float64
	flLatencyP99  float64
	flLatencyP95  float64
	flLatencyP50  float64
)

func init() {
	flag.StringVar(&flLoggingLevel, "verbosity", "info", "the logging level (e.g. debug)")
	flag.BoolVar(&flCLI, "cli", false, "run as CLI application to manage rollout in intervals")
	flag.StringVar(&flHTTPAddr, "http-addr", "", "listen on http portrun on request (e.g. :8080)")
	flag.StringVar(&flProject, "project", "", "project in which the service is deployed")
	flag.StringVar(&flLabelSelector, "label", "rollout-strategy=gradual", "filter services based on a label (e.g. team=backend)")
	flag.StringVar(&flRegionsString, "regions", "", "the Cloud Run regions where the service should be looked at")
	flag.Var(&flSteps, "step", "a percentage in traffic the candidate should go through")
	flag.StringVar(&flStepsString, "steps", "5,20,50,80", "define steps in one flag separated by commas (e.g. 5,30,60)")
	flag.Int64Var(&flInterval, "interval", 0, "the time between each rollout step")
	flag.Float64Var(&flErrorRate, "max-error-rate", 1.0, "expected max server error rate (in percent)")
	flag.Float64Var(&flLatencyP99, "latency-p99", 0, "expected max latency for 99th percentile of requests (set 0 to ignore)")
	flag.Float64Var(&flLatencyP95, "latency-p95", 0, "expected max latency for 95th percentile of requests (set 0 to ignore)")
	flag.Float64Var(&flLatencyP50, "latency-p50", 0, "expected max latency for 50th percentile of requests (set 0 to ignore)")
	flag.Parse()

	if flRegionsString != "" {
		flRegions = strings.Split(flRegionsString, ",")
	}
}

func main() {
	logger := logrus.New()
	loggingLevel, err := logrus.ParseLevel(flLoggingLevel)
	if err != nil {
		logger.Fatalf("invalid logging level: %v", err)
	}
	logger.SetLevel(loggingLevel)

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		logger.Formatter = stackdriverFormatter.NewFormatter(
			stackdriverFormatter.WithService(os.Getenv("K_SERVICE")),
		)
	}

	valid, err := flagsAreValid()
	if !valid {
		logger.Fatalf("invalid flags: %v", err)
	}

	// Configuration.
	target := config.NewTarget(flProject, flRegions, flLabelSelector)
	healthCriteria := healthCriteriaFromFlags(flErrorRate, flLatencyP99, flLatencyP95, flLatencyP50)
	printHealthCriteria(logger, healthCriteria)
	cfg := config.WithValues([]*config.Target{target}, flSteps, flInterval, healthCriteria)
	if !cfg.IsValid(flCLI) {
		logger.Fatalf("invalid rollout configuration")
	}

	ctx := context.Background()
	metricsProvider, err := stackdriver.NewAPIClient(ctx, flProject)
	if err != nil {
		logger.Fatalf("failed to initialize metrics provider: %v", err)
	}

	if flCLI {
		runCLI(ctx, logger, metricsProvider, cfg)
	}
}

func runCLI(ctx context.Context, logger *logrus.Logger, metricsProvider metrics.Metrics, cfg *config.Config) {
	for {
		services, err := getTargetedServices(ctx, logger, cfg.Targets)
		if err != nil {
			log.Fatalf("failed to get targeted services %v", err)
		}
		if len(services) == 0 {
			logger.Warn("no service matches the targets")
		}

		// TODO: Handle all the filtered services
		client, err := runapi.NewAPIClient(ctx, services[0].Region)
		if err != nil {
			logger.Fatal("failed to initialize Cloud Run API client")
		}
		roll := rollout.New(client, metricsProvider, services[0], cfg.Strategy).WithLogger(logger)

		changed, err := roll.Rollout()
		if err != nil {
			logger.Fatalf("rollout failed: %v", err)
		}
		if changed {
			logger.Info("rollout process succeeded")
		}

		duration := time.Duration(cfg.Strategy.Interval)
		time.Sleep(duration * time.Second)
	}
}

func flagsAreValid() (bool, error) {
	// -steps flag has precedence over the list of -step flags.
	if flStepsString != "" {
		steps := strings.Split(flStepsString, ",")
		flSteps = []int64{}
		for _, step := range steps {
			value, err := strconv.ParseInt(step, 10, 64)
			if err != nil {
				return false, errors.Wrap(err, "invalid step value")
			}

			flSteps = append(flSteps, value)
		}
	}

	if !flCLI && flHTTPAddr == "" {
		return false, errors.New("one of -cli or -http-addr must be set")
	}
	if flCLI && flHTTPAddr != "" {
		return false, errors.New("only one of -cli or -http-addr can be used")
	}

	for _, region := range flRegions {
		if region == "" {
			return false, errors.New("region cannot be empty")
		}
	}

	return true, nil
}

// healthCriteriaFromFlags checks the metrics-related flags and return an array
// of config.Metric based on them.
func healthCriteriaFromFlags(errorRate, latencyP99, latencyP95, latencyP50 float64) []config.Metric {
	metrics := []config.Metric{
		{Type: config.ErrorRateMetricsCheck, Threshold: errorRate},
	}

	if latencyP99 > 0 {
		metrics = append(metrics, config.Metric{Type: config.LatencyMetricsCheck, Percentile: 99, Threshold: latencyP99})
	}
	if latencyP95 > 0 {
		metrics = append(metrics, config.Metric{Type: config.LatencyMetricsCheck, Percentile: 95, Threshold: latencyP95})
	}
	if latencyP50 > 0 {
		metrics = append(metrics, config.Metric{Type: config.LatencyMetricsCheck, Percentile: 50, Threshold: latencyP50})
	}

	return metrics
}

func printHealthCriteria(logger *logrus.Logger, healthCriteria []config.Metric) {
	for _, criteria := range healthCriteria {
		lg := logger.WithFields(logrus.Fields{
			"metricsType": criteria.Type,
			"threshold":   criteria.Threshold,
		})

		switch criteria.Type {
		case config.ErrorRateMetricsCheck:
			lg.Debug("found health criterion")
			break
		case config.LatencyMetricsCheck:
			lg.WithField("percentile", criteria.Percentile).Debug("found health criterion")
			break
		default:
			lg.Debug("invalid health criterion")
		}
	}
}
