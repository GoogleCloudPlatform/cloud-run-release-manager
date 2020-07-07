package run

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"google.golang.org/api/option"
	"google.golang.org/api/run/v1"
)

// Client represents a wrapper around the Cloud Run package.
type Client interface {
	Service(namespace, serviceID string) (*run.Service, error)
	ReplaceService(namespace, serviceID string, svc *run.Service) (*run.Service, error)
}

// API is a wrapper for the Cloud Run package.
type API struct {
	Client *run.APIService
	Region string
}

// NewAPIClient initializes an instance of APIService.
func NewAPIClient(ctx context.Context, region string) (*API, error) {
	regionalEndpoint := fmt.Sprintf("https://%s-run.googleapis.com/", region)
	client, err := run.NewService(ctx, option.WithEndpoint(regionalEndpoint))
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize client for the Cloud Run API")
	}

	return &API{
		Client: client,
		Region: region,
	}, nil
}

// Service retrieves information about a service.
func (a *API) Service(namespace, serviceID string) (*run.Service, error) {
	serviceName := serviceName(namespace, serviceID)
	return a.Client.Namespaces.Services.Get(serviceName).Do()
}

// ReplaceService replaces an existing service.
func (a *API) ReplaceService(namespace, serviceID string, svc *run.Service) (*run.Service, error) {
	serviceName := serviceName(namespace, serviceID)
	return a.Client.Namespaces.Services.ReplaceService(serviceName, svc).Do()
}

// generateServiceName returns the name of the specified service. It returns the
// form namespaces/{namespace_id}/services/{service_id}.
//
// For Cloud Run (fully managed), the namespace is the project ID or number.
func serviceName(namespace, serviceID string) string {
	return fmt.Sprintf("namespaces/%s/services/%s", namespace, serviceID)
}
