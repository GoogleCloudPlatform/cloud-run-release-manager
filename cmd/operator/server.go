package main

import (
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/sirupsen/logrus"
)

// makeRolloutHandler creates a request handler to perform a rollout process.
func makeRolloutHandler(logger *logrus.Logger, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		// TODO(gvso): Handle all the strategies.
		errs := runRollouts(ctx, logger, cfg.Strategies[0])
		errsStr := rolloutErrsToString(errs)
		if len(errs) != 0 {
			msg := fmt.Sprintf("there were %d errors: \n%s", len(errs), errsStr)
			logger.Warn(msg)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, msg)
		}
	}
}
