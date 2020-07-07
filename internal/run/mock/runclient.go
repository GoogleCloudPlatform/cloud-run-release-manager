package mock

import "google.golang.org/api/run/v1"

// RunAPI represents a mock implementation of run.API.
type RunAPI struct {
	ServiceFn      func(namespace, serviceID string) (*run.Service, error)
	ServiceInvoked bool

	ReplaceServiceFn      func(namespace, serviceID string, svc *run.Service) (*run.Service, error)
	ReplaceServiceInvoked bool
}

// Service invokes the mock implementation and marks the function as invoked.
func (a *RunAPI) Service(namespace, service string) (*run.Service, error) {
	a.ServiceInvoked = true
	return a.ServiceFn(namespace, service)
}

// ReplaceService invokes the mock implementation and marks the function as invoked.
func (a *RunAPI) ReplaceService(namespace, serviceID string, svc *run.Service) (*run.Service, error) {
	a.ReplaceServiceInvoked = true
	return a.ReplaceServiceFn(namespace, serviceID, svc)
}
