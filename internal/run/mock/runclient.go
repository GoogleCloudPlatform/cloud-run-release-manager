package mock

import "google.golang.org/api/run/v1"

// RunAPI represents a mock implementation of run.API.
type RunAPI struct {
	ServiceFn      func(name string) (*run.Service, error)
	ServiceInvoked bool

	ReplaceServiceFn      func(name string, svc *run.Service) (*run.Service, error)
	ReplaceServiceInvoked bool
}

// Service invokes the mock implementation and marks the function as invoked.
func (a *RunAPI) Service(name string) (*run.Service, error) {
	a.ServiceInvoked = true
	return a.ServiceFn(name)
}

// ReplaceService invokes the mock implementation and marks the function as invoked.
func (a *RunAPI) ReplaceService(name string, svc *run.Service) (*run.Service, error) {
	a.ReplaceServiceInvoked = true
	return a.ReplaceServiceFn(name, svc)
}
