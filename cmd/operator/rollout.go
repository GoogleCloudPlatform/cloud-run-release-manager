package main

import (
	"context"
	"fmt"
	"sync"

	runapi "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/run"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/rollout"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// runRollouts concurrently handles the rollout of the targeted services.
func runRollouts(ctx context.Context, logger *logrus.Logger, cfg *config.Config) []error {
	svcs, err := getTargetedServices(ctx, logger, cfg.Targets)
	if err != nil {
		return []error{errors.Wrap(err, "failed to get targeted services")}
	}
	if len(svcs) == 0 {
		logger.Warn("no service matches the targets")
	}

	var (
		errs []error
		mu   sync.Mutex
		wg   sync.WaitGroup
	)
	for _, svc := range svcs {
		wg.Add(1)
		go func(ctx context.Context, lg *logrus.Logger, svc *rollout.ServiceRecord, strategy *config.Strategy) {
			defer wg.Done()
			err := handleRollout(ctx, lg, svc, strategy)
			if err != nil {
				lg.Debugf("rollout error for service %q: %+v", svc.Service.Metadata.Name, err)
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(ctx, logger, svc, cfg.Strategy)
	}
	wg.Wait()

	return errs
}

// handleRollout manages the rollout process for a single service.
func handleRollout(ctx context.Context, logger *logrus.Logger, service *rollout.ServiceRecord, strategy *config.Strategy) error {
	lg := logger.WithFields(logrus.Fields{
		"project": service.Project,
		"service": service.Metadata.Name,
		"region":  service.Region,
	})

	client, err := runapi.NewAPIClient(ctx, service.Region)
	if err != nil {
		return errors.Wrap(err, "failed to initialize Cloud Run API client")
	}
	metricsProvider, err := chooseMetricsProvider(ctx, lg, service.Project, service.Region, service.Metadata.Name)
	if err != nil {
		return errors.Wrap(err, "failed to initialize metrics provider")
	}
	roll := rollout.New(ctx, metricsProvider, service, strategy).WithClient(client).WithLogger(lg.Logger)

	changed, err := roll.Rollout()
	if err != nil {
		lg.Errorf("rollout failed, error=%v", err)
		return errors.Wrap(err, "rollout failed")
	}

	if changed {
		lg.Info("service was successfully updated")
	} else {
		lg.Debug("service kept unchanged")
	}
	return nil
}

// rolloutErrsToString returns the string representation of all the errors found
// during the rollout of all targeted services.
func rolloutErrsToString(errs []error) (errsStr string) {
	for i, err := range errs {
		if i > 0 {
			errsStr += "\n"
		}
		errsStr += fmt.Sprintf("[error#%d] %v", i, err)
	}
	return errsStr
}
