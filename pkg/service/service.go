package service

import (
	runapi "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/run"
)

// Client is the connection to update and obtain data about a single service.
type Client struct {
	RunClient   runapi.Client
	Project     string
	ServiceName string
	Region      string
}
