package main

import (
	"context"
	"sync"

	runapi "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/run"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/rollout"
	"github.com/pkg/errors"
	"google.golang.org/api/run/v1"
)

// getTargetedServices returned a list of service records that match the target
// configuration.
func getTargetedServices(ctx context.Context, targets []*config.Target) ([]*rollout.ServiceRecord, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		retServices []*rollout.ServiceRecord
		retError    error
		mu          sync.Mutex
		wg          sync.WaitGroup
	)

	for _, target := range targets {
		regions, err := determineRegions(ctx, target)
		if err != nil {
			return nil, errors.Wrap(err, "cannot determine regions")
		}

		for _, region := range regions {
			wg.Add(1)

			go func(ctx context.Context, region, labelSelector string) {
				defer wg.Done()
				svcs, err := getServicesByRegionAndLabel(ctx, target.Project, region, target.LabelSelector)
				if err != nil {
					retError = err
					cancel()
					return
				}

				for _, svc := range svcs {
					mu.Lock()
					retServices = append(retServices, newServiceRecord(svc, target.Project, region))
					mu.Unlock()
				}

			}(ctx, region, target.LabelSelector)
		}
	}

	wg.Wait()
	return retServices, retError
}

// getServicesByRegionAndLabel returns all the service records that match the
// labelSelector in a specific region.
func getServicesByRegionAndLabel(ctx context.Context, project, region, labelSelector string) ([]*run.Service, error) {
	runclient, err := runapi.NewAPIClient(ctx, region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize Cloud Run client")
	}

	svcs, err := runclient.ServicesWithLabelSelector(project, labelSelector)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get services with label %q in region %q", labelSelector, region)
	}

	return svcs, nil
}

// determineRegions gets the regions the label selector should be searched at.
//
// If the target configuration does not specify any regions, the entire list of
// regions is retrieved from API.
func determineRegions(ctx context.Context, target *config.Target) ([]string, error) {
	regions := target.Regions
	if len(regions) != 0 {
		return regions, nil
	}

	regions, err := runapi.Regions(ctx, target.Project)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot get list of regions from Cloud Run API")
	}
	return regions, nil
}

// newServiceRecord creates a new service record.
func newServiceRecord(svc *run.Service, project, region string) *rollout.ServiceRecord {
	return &rollout.ServiceRecord{
		Service: svc,
		Project: project,
		Region:  region,
	}
}
