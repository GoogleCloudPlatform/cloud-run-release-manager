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
	Service(name string) (*run.Service, error)
	ReplaceService(name string, svc *run.Service) (*run.Service, error)
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
		return nil, errors.Wrap(err, "could not initialize a APIService instance")
	}

	return &API{
		Client: client,
		Region: region,
	}, nil
}

// Service retrieves information about a service.
func (a *API) Service(name string) (*run.Service, error) {
	return a.Client.Namespaces.Services.Get(name).Do()
}

// ReplaceService replaces an existing service.
func (a *API) ReplaceService(name string, svc *run.Service) (*run.Service, error) {
	return a.Client.Namespaces.Services.ReplaceService(name, svc).Do()
}
