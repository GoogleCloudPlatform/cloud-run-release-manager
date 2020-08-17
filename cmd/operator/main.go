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
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/config"
	sdlog "github.com/TV4/logrus-stackdriver-formatter"
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
	flLoggingLevel    string
	flCLI             bool
	flCLILoopInterval time.Duration
	flHTTPAddr        string
	flProject         string
	flLabelSelector   string

	// Empty array means all regions.
	flRegions       []string
	flRegionsString string

	// Rollout strategy-related flags.
	flSteps              stepFlags
	flStepsString        string
	flHealthOffset       time.Duration
	flTimeBeweenRollouts time.Duration
	flMinRequestCount    int
	flErrorRate          float64
	flLatencyP99         float64
	flLatencyP95         float64
	flLatencyP50         float64

	// Metrics provider flags.
	flGoogleSheetsID string
)

func init() {
	defaultAddr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		defaultAddr = fmt.Sprintf(":%s", v)
	}

	flag.StringVar(&flLoggingLevel, "verbosity", "info", "the logging level (e.g. debug)")
	flag.BoolVar(&flCLI, "cli", false, "run as CLI application to manage rollout in intervals")
	flag.DurationVar(&flCLILoopInterval, "cli-run-interval", 60*time.Second, "the time between each rollout process (in seconds)")
	flag.StringVar(&flHTTPAddr, "http-addr", defaultAddr, "address where to listen to http requests (e.g. :8080)")
	flag.StringVar(&flProject, "project", "", "project in which the service is deployed")
	flag.StringVar(&flLabelSelector, "label", "rollout-strategy=gradual", "filter services based on a label (e.g. team=backend)")
	flag.StringVar(&flRegionsString, "regions", "", "the Cloud Run regions where the services should be looked at")
	flag.Var(&flSteps, "step", "a percentage in traffic the candidate should go through")
	flag.StringVar(&flStepsString, "steps", "5,20,50,80", "define steps in one flag separated by commas (e.g. 5,30,60)")
	flag.DurationVar(&flHealthOffset, "healthcheck-offset", 30*time.Minute, "time window to look back during health check to assess the candidate's health")
	flag.DurationVar(&flTimeBeweenRollouts, "min-wait", 30*time.Minute, "minimum time to wait between rollout stages (in minutes), use 0 to disable")
	flag.IntVar(&flMinRequestCount, "min-requests", 100, "expected minimum requests (in time window given by -healthcheck-offset) needed to determine candidate's health")
	flag.Float64Var(&flErrorRate, "max-error-rate", 1.0, "expected max server error rate (in percent)")
	flag.Float64Var(&flLatencyP99, "latency-p99", 0, "expected max latency for 99th percentile of requests (set 0 to ignore)")
	flag.Float64Var(&flLatencyP95, "latency-p95", 0, "expected max latency for 95th percentile of requests (set 0 to ignore)")
	flag.Float64Var(&flLatencyP50, "latency-p50", 0, "expected max latency for 50th percentile of requests (set 0 to ignore)")
	flag.StringVar(&flGoogleSheetsID, "google-sheets", "", "ID of public Google sheets document to use as metrics provider")
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
		serviceName := os.Getenv("K_SERVICE")
		if serviceName == "" {
			serviceName = "cloud-run-release-operator"
		}
		logger.Formatter = sdlog.NewFormatter(
			sdlog.WithService(serviceName),
		)
	}

	if flProject == "" {
		logger.Info("-project not specified, trying to autodetect one")
		flProject, err = determineProjectID()
		if err != nil {
			logger.Fatalf("failed to detect project, must specify one with -project: %v", err)
		} else {
			logger.Infof("project detected: %s", flProject)
		}
	}

	if err := validateFlags(); err != nil {
		logger.Fatalf("invalid flags: %v", err)
	}

	// Configuration.
	target := config.NewTarget(flProject, flRegions, flLabelSelector)
	healthCriteria := healthCriteriaFromFlags(flMinRequestCount, flErrorRate, flLatencyP99, flLatencyP95, flLatencyP50)
	printHealthCriteria(logger, healthCriteria)
	strategy := config.NewStrategy(target, flSteps, flHealthOffset, flTimeBeweenRollouts, healthCriteria)
	cfg := &config.Config{Strategies: []config.Strategy{strategy}}
	if err := cfg.Validate(); err != nil {
		logger.Fatalf("invalid rollout configuration: %v", err)
	}

	ctx := context.Background()
	if flCLI {
		runDaemon(ctx, logger, cfg)
	} else {
		http.HandleFunc("/rollout", makeRolloutHandler(logger, cfg))
		logger.WithField("addr", flHTTPAddr).Infof("starting server")
		logger.Fatal(http.ListenAndServe(flHTTPAddr, nil))
	}
}

func runDaemon(ctx context.Context, logger *logrus.Logger, cfg *config.Config) {
	for {
		// TODO(gvso): Handle all the strategies.
		errs := runRollouts(ctx, logger, cfg.Strategies[0])
		errsStr := rolloutErrsToString(errs)
		if len(errs) != 0 {
			logger.Warnf("there were %d errors: \n%s", len(errs), errsStr)
		}

		time.Sleep(flCLILoopInterval)
	}
}

func validateFlags() error {
	// -steps flag has precedence over the list of -step flags.
	if flStepsString != "" {
		steps := strings.Split(flStepsString, ",")
		flSteps = []int64{}
		for _, step := range steps {
			value, err := strconv.ParseInt(step, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "invalid step value %v", value)
			}

			flSteps = append(flSteps, value)
		}
	}

	for _, region := range flRegions {
		if region == "" {
			return errors.New("regions cannot be empty")
		}
	}

	if flCLILoopInterval < 0 {
		return errors.Errorf("cli run interval cannot be negative, got %s", flCLILoopInterval)
	}
	return nil
}

// healthCriteriaFromFlags checks the metrics-related flags and return an array
// of config.Metric based on them.
func healthCriteriaFromFlags(requestCount int, errorRate, latencyP99, latencyP95, latencyP50 float64) []config.HealthCriterion {
	metrics := []config.HealthCriterion{
		{Metric: config.ErrorRateMetricsCheck, Threshold: errorRate},
		{Metric: config.RequestCountMetricsCheck, Threshold: float64(requestCount)},
	}

	if latencyP99 > 0 {
		metrics = append(metrics, config.HealthCriterion{Metric: config.LatencyMetricsCheck, Percentile: 99, Threshold: latencyP99})
	}
	if latencyP95 > 0 {
		metrics = append(metrics, config.HealthCriterion{Metric: config.LatencyMetricsCheck, Percentile: 95, Threshold: latencyP95})
	}
	if latencyP50 > 0 {
		metrics = append(metrics, config.HealthCriterion{Metric: config.LatencyMetricsCheck, Percentile: 50, Threshold: latencyP50})
	}

	return metrics
}

func printHealthCriteria(logger *logrus.Logger, healthCriteria []config.HealthCriterion) {
	for _, criteria := range healthCriteria {
		lg := logger.WithFields(logrus.Fields{
			"metricsType": criteria.Metric,
			"threshold":   criteria.Threshold,
		})

		if criteria.Metric == config.LatencyMetricsCheck {
			lg = lg.WithField("percentile", criteria.Percentile)
		}
		lg.Debug("found health criterion")
	}
}

func determineProjectID() (string, error) {
	if metadata.OnGCE() {
		v, err := metadata.ProjectID()
		if err != nil {
			return "", errors.Wrapf(err, "error when getting project ID from compute metadata")
		}
		return v, nil
	}

	// Try to get project ID by retrieving default value in gcloud.
	cmd := exec.Command("gcloud", "config", "get-value", "core/project", "-q")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		msg := "error when running gcloud command to get default project"
		if stderr.Len() != 0 {
			msg += fmt.Sprintf(", stderr=%s", stderr.String())
		}
		return "", errors.Wrapf(err, msg)
	}

	v := strings.TrimSpace(stdout.String())
	if v == "" {
		return "", errors.New("gcloud command returned empty project value")
	}
	return v, nil
}
