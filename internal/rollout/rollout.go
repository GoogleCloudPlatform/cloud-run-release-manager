package rollout

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/health"
	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/metrics"
	runapi "github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/run"
	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/util"
	"github.com/jonboulle/clockwork"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/run/v1"
)

// Annotations name for information related to the rollout.
const (
	StableRevisionAnnotation              = "rollout.cloud.run/stableRevision"
	CandidateRevisionAnnotation           = "rollout.cloud.run/candidateRevision"
	LastFailedCandidateRevisionAnnotation = "rollout.cloud.run/lastFailedCandidateRevision"
	LastRolloutAnnotation                 = "rollout.cloud.run/lastRollout"
	LastHealthReportAnnotation            = "rollout.cloud.run/lastHealthReport"
)

// ServiceRecord holds a service object and information about it.
type ServiceRecord struct {
	*run.Service
	Project string
	Region  string
}

// Rollout is the rollout manager.
type Rollout struct {
	ctx             context.Context
	metricsProvider metrics.Provider
	service         *run.Service
	serviceName     string
	project         string
	region          string
	strategy        config.Strategy
	runClient       runapi.Client
	log             *logrus.Entry
	time            clockwork.Clock

	// Used to determine if candidate should become stable during update.
	promoteToStable bool

	// Used to update annotations when rollout should occur.
	shouldRollout bool

	// Used to update annotations when rollback should occur.
	shouldRollback bool
}

// Automatic tags.
const (
	StableTag    = "stable"
	CandidateTag = "candidate"
	LatestTag    = "latest"
)

// New returns a new rollout manager.
func New(ctx context.Context, metricsProvider metrics.Provider, svcRecord *ServiceRecord, strategy config.Strategy) *Rollout {
	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	return &Rollout{
		ctx:             ctx,
		metricsProvider: metricsProvider,
		service:         svcRecord.Service,
		serviceName:     svcRecord.Metadata.Name,
		project:         svcRecord.Project,
		region:          svcRecord.Region,
		strategy:        strategy,
		log:             logrus.NewEntry(logrus.New()),
		time:            clockwork.NewRealClock(),
	}
}

// WithClient updates the client in the rollout instance.
func (r *Rollout) WithClient(client runapi.Client) *Rollout {
	r.runClient = client
	return r
}

// WithLogger updates the logger in the rollout instance.
func (r *Rollout) WithLogger(logger *logrus.Logger) *Rollout {
	r.log = logger.WithField("project", r.project)
	return r
}

// WithClock updates the clock in the rollout instance.
func (r *Rollout) WithClock(clock clockwork.Clock) *Rollout {
	r.time = clock
	return r
}

// Rollout handles the gradual rollout.
func (r *Rollout) Rollout() (bool, error) {
	r.log = r.log.WithFields(logrus.Fields{
		"project": r.project,
		"service": r.serviceName,
		"region":  r.region,
	})

	_, trafficChanged, err := r.UpdateService(r.service)
	if err != nil {
		return false, errors.Wrapf(err, "failed to perform rollout")
	}

	// Service is non-nil only when the replacement of the service succeded.
	return trafficChanged, nil
}

// UpdateService changes the traffic configuration for the revisions and update
// the service.
//
// If successful, it always returns an updated service object (with changes in
// the traffic and/or annotations) or an unchanged service object if no stable
// or candidate revision was found.
// If the traffic configuration changed, the second return value is set to true.
func (r *Rollout) UpdateService(svc *run.Service) (*run.Service, bool, error) {
	stable := DetectStableRevisionName(svc)
	if stable == "" {
		r.log.Info("cannot find a stable revision (that gets 100% of the traffic)")
		return svc, false, nil
	}

	candidate := DetectCandidateRevisionName(svc, stable)
	if candidate == "" {
		r.log.Debug("currently no candidate revision exists to rollout")
		return svc, false, nil
	}
	r.log = r.log.WithFields(logrus.Fields{"stable": stable, "candidate": candidate})

	// A new candidate does not have metrics yet, so it can't be diagnosed.
	if isNewCandidate(svc, candidate) {
		r.log.Debug("new candidate, assign some traffic")
		r.shouldRollout = true
		svc.Spec.Traffic = r.rollForwardTraffic(svc.Spec.Traffic, stable, candidate)
		svc = r.updateAnnotations(svc, stable, candidate)
		r.setHealthReportAnnotation(svc, "new candidate, no health report available yet")

		err := r.replaceService(svc)
		return svc, true, errors.Wrap(err, "failed to replace service")
	}

	diagnosis, err := r.diagnoseCandidate(candidate, r.strategy.HealthCriteria)
	if err != nil {
		r.log.Error("could not diagnose candidate's health")
		return svc, false, errors.Wrapf(err, "failed to diagnose health for candidate %q", candidate)
	}

	traffic, trafficChanged, err := r.determineTraffic(svc, diagnosis.OverallResult, stable, candidate)
	if err != nil {
		return svc, false, errors.Wrap(err, "failed to configure traffic after diagnosis")
	}

	svc.Spec.Traffic = traffic
	svc = r.updateAnnotations(svc, stable, candidate)

	// If candidate is healthy, traffic only changes when enough time has
	// elapsed. Thus, we can pass it as an argument representing if enough time
	// has elapsed since last rollout.
	report := health.StringReport(r.strategy.HealthCriteria, diagnosis, trafficChanged)
	r.setHealthReportAnnotation(svc, report)

	err = r.replaceService(svc)
	return svc, trafficChanged, errors.Wrap(err, "failed to replace service")
}

// replaceService updates the service object in Cloud Run.
func (r *Rollout) replaceService(svc *run.Service) error {
	_, err := r.runClient.ReplaceService(r.project, r.serviceName, svc)
	return errors.Wrapf(err, "could not update service %q", r.serviceName)
}

// updateAnnotations updates the annotations to keep some state about the rollout.
func (r *Rollout) updateAnnotations(svc *run.Service, stable, candidate string) *run.Service {
	if r.shouldRollout {
		now := r.time.Now().Format(time.RFC3339)
		setAnnotation(svc, LastRolloutAnnotation, now)
	}

	// The candidate has become the stable revision.
	if r.promoteToStable {
		setAnnotation(svc, StableRevisionAnnotation, candidate)
		delete(svc.Metadata.Annotations, CandidateRevisionAnnotation)
		return svc
	}

	setAnnotation(svc, StableRevisionAnnotation, stable)
	setAnnotation(svc, CandidateRevisionAnnotation, candidate)
	if r.shouldRollback {
		setAnnotation(svc, LastFailedCandidateRevisionAnnotation, candidate)
	}

	return svc
}

// setAnnotation sets the value of an annotation.
func setAnnotation(svc *run.Service, key, value string) {
	if svc.Metadata.Annotations == nil {
		svc.Metadata.Annotations = make(map[string]string)
	}
	svc.Metadata.Annotations[key] = value
}

// setHealthReportAnnotation appends the current time to the report and sets
// the health report annotation.
func (r *Rollout) setHealthReportAnnotation(svc *run.Service, report string) {
	report += fmt.Sprintf("\nlastUpdate: %s", r.time.Now().Format(time.RFC3339))
	setAnnotation(svc, LastHealthReportAnnotation, report)
}

// diagnoseCandidate returns the candidate's diagnosis based on metrics.
func (r *Rollout) diagnoseCandidate(candidate string, healthCriteria []config.HealthCriterion) (d health.Diagnosis, err error) {
	r.log.Debug("collecting metrics from API")
	ctx := util.ContextWithLogger(r.ctx, r.log)
	r.metricsProvider.SetCandidateRevision(candidate)
	healthCheckOffset := r.strategy.HealthCheckOffset
	metricsValues, err := health.CollectMetrics(ctx, r.metricsProvider, healthCheckOffset, healthCriteria)
	if err != nil {
		return d, errors.Wrap(err, "failed to collect metrics")
	}

	r.log.Debug("diagnosing candidate's health")
	d, err = health.Diagnose(ctx, healthCriteria, metricsValues)
	return d, errors.Wrap(err, "failed to diagnose candidate's health")
}
