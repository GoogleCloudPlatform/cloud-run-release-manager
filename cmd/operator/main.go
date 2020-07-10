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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/run"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/rollout"
	stackdriver "github.com/TV4/logrus-stackdriver-formatter"
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
	flCLI         bool
	flHTTPAddr    string
	flProject     string
	flRegion      string
	flService     string
	flSteps       stepFlags
	flStepsString string
	flInterval    int64
)

func init() {
	flag.BoolVar(&flCLI, "cli", false, "run as CLI application to manage rollout in intervals")
	flag.StringVar(&flHTTPAddr, "http-addr", "", "listen on http portrun on request (e.g. :8080)")
	flag.StringVar(&flProject, "project", "", "project in which the service is deployed")
	flag.StringVar(&flRegion, "region", "", "the Cloud Run region where the service is deployed")
	flag.StringVar(&flService, "service", "", "the service to manage")
	flag.Var(&flSteps, "step", "a percentage in traffic the candidate should go through")
	flag.StringVar(&flStepsString, "steps", "", "define steps in one flag separated by commas (e.g. 5,30,60)")
	flag.Int64Var(&flInterval, "interval", 0, "the time between each rollout step")
	flag.Parse()
}

func main() {
	logger := logrus.New()

	// -steps flag has precedence over the list of -step flags.
	if flStepsString != "" {
		steps := strings.Split(flStepsString, ",")
		flSteps = []int64{}
		for _, step := range steps {
			value, err := strconv.ParseInt(step, 10, 64)
			if err != nil {
				logger.Fatalf("invalid step value: %v", err)
			}

			flSteps = append(flSteps, value)
		}
	}

	if !flCLI && flHTTPAddr == "" {
		logger.Fatal("one of -cli or -http-addr must be set")
	}

	if flCLI && flHTTPAddr != "" {
		logger.Fatal("only one of -cli or -http-addr can be used")
	}

	cfg := config.WithValues(flProject, flRegion, flService, flSteps, flInterval)
	if !cfg.IsValid(flCLI) {
		logger.Fatalf("invalid config values")
	}

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		logger.Formatter = stackdriver.NewFormatter(
			stackdriver.WithService("cloud-run-release-operator"),
		)
	}

	if flCLI {
		runCLI(logger, cfg)
	}
}

func runCLI(logger *logrus.Logger, cfg *config.Config) {

	client, err := run.NewAPIClient(context.Background(), cfg.Metadata.Region)
	if err != nil {
		logger.Fatalf("could not initilize Cloud Run client: %v", err)
	}
	roll := rollout.New(client, cfg).WithLogger(logger)

	for {
		changed, err := roll.Rollout()
		if err != nil {
			logger.Infof("Rollout failed: %v", err)
		}
		if changed {
			logger.Info("Rollout process succeeded")
		}

		interval := time.Duration(cfg.Rollout.Interval)
		time.Sleep(interval * time.Second)
	}
}
