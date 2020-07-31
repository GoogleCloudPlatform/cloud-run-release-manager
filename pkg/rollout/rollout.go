package rollout

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/metrics"
	runapi "github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/run"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/internal/util"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/config"
	"github.com/GoogleCloudPlatform/cloud-run-release-operator/pkg/health"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/run/v1"
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
	strategy        *config.Strategy
	runClient       runapi.Client
	log             *logrus.Entry

	// Used to determine if candidate should become stable during update.
	promoteToStable bool

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
func New(ctx context.Context, metricsProvider metrics.Provider, svcRecord *ServiceRecord, strategy *config.Strategy) *Rollout {
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

// Rollout handles the gradual rollout.
func (r *Rollout) Rollout() (bool, error) {
	r.log = r.log.WithFields(logrus.Fields{
		"project": r.project,
		"service": r.serviceName,
		"region":  r.region,
	})

	svc, err := r.UpdateService(r.service)
	if err != nil {
		return false, errors.Wrapf(err, "failed to perform rollout")
	}

	// Service is non-nil only when the replacement of the service succeded.
	return (svc != nil), nil
}

// UpdateService changes the traffic configuration for the revisions and update
// the service.
func (r *Rollout) UpdateService(svc *run.Service) (*run.Service, error) {
	stable := DetectStableRevisionName(svc)
	if stable == "" {
		r.log.Info("could not determine stable revision")
		return nil, nil
	}

	candidate := DetectCandidateRevisionName(svc, stable)
	if candidate == "" {
		r.log.Info("could not determine candidate revision")
		return nil, nil
	}
	r.log = r.log.WithFields(logrus.Fields{"stable": stable, "candidate": candidate})

	// A new candidate does not have metrics yet, so it can't be diagnosed.
	if isNewCandidate(svc, candidate) {
		r.log.Debug("new candidate, assign some traffic")
		svc = r.PrepareRollForward(svc, stable, candidate)
		svc = r.updateAnnotations(svc, stable, candidate)
		err := r.replaceService(svc)
		return svc, errors.Wrap(err, "failed to replace service")
	}

	diagnosis, err := r.diagnoseCandidate(candidate, r.strategy.Metrics)
	if err != nil {
		r.log.Error("could not diagnose candidate's health")
		return nil, errors.Wrapf(err, "failed to diagnose health for candidate %q", candidate)
	}

	switch diagnosis.OverallResult {
	case health.Inconclusive:
		r.log.Debug("health check inconclusive")
		return nil, nil
	case health.Healthy:
		r.log.Debug("healthy candidate, roll forward")
		svc = r.PrepareRollForward(svc, stable, candidate)
		break
	case health.Unhealthy:
		r.log.Info("unhealthy candidate, rollback")
		r.shouldRollback = true
		svc = r.PrepareRollback(svc, stable, candidate)
		break
	default:
		return nil, errors.Errorf("invalid candidate's health diagnosis %v", diagnosis.OverallResult)
	}

	// TODO(gvso): include annotation about the diagnosis (especially when
	// diagnosis is unhealthy).
	svc = r.updateAnnotations(svc, stable, candidate)
	err = r.replaceService(svc)
	return svc, errors.Wrap(err, "failed to replace service")
}

// PrepareRollForward changes the traffic configuration of the service to
// increase the traffic to the candidate.
//
// It creates a new traffic configuration for the service. It creates a new
// traffic configuration for the candidate and stable revisions.
// The method respects user-defined revision tags.
func (r *Rollout) PrepareRollForward(svc *run.Service, stable, candidate string) *run.Service {
	r.log.Debug("splitting traffic")

	var traffic []*run.TrafficTarget
	var stablePercent int64

	candidateTraffic, promoteCandidateToStable := r.newCandidateTraffic(svc, candidate)
	if promoteCandidateToStable {
		r.promoteToStable = true
		candidateTraffic.Tag = StableTag
	} else {
		// If candidate is not being promoted, also include traffic
		// configuration for stable revision.
		stablePercent = 100 - candidateTraffic.Percent
		stableTraffic := newTrafficTarget(stable, stablePercent, StableTag)
		traffic = append(traffic, stableTraffic)
	}
	traffic = append(traffic, candidateTraffic)
	traffic = append(traffic, inheritRevisionTags(svc)...)

	if r.promoteToStable {
		r.log.Infof("will make candidate stable")
	} else {
		r.log.WithFields(logrus.Fields{
			"stablePercent":    stablePercent,
			"candidatePercent": candidateTraffic.Percent,
		}).Info("set traffic split")
	}

	svc.Spec.Traffic = traffic
	return svc
}

// PrepareRollback redirects all the traffic to the stable revision.
func (r *Rollout) PrepareRollback(svc *run.Service, stable, candidate string) *run.Service {
	traffic := []*run.TrafficTarget{
		newTrafficTarget(stable, 100, StableTag),
		newTrafficTarget(candidate, 0, CandidateTag),
	}
	traffic = append(traffic, inheritRevisionTags(svc)...)

	svc.Spec.Traffic = traffic
	return svc
}

// replaceService updates the service object in Cloud Run.
func (r *Rollout) replaceService(svc *run.Service) error {
	_, err := r.runClient.ReplaceService(r.project, r.serviceName, svc)
	return errors.Wrapf(err, "could not update service %q", r.serviceName)
}

// newCandidateTraffic returns the next candidate's traffic configuration.
//
// It also checks if the candidate should be promoted to stable in the next
// update and returns a boolean about that.
func (r *Rollout) newCandidateTraffic(svc *run.Service, candidate string) (*run.TrafficTarget, bool) {
	var promoteToStable bool
	var candidatePercent int64
	candidateTarget := r.currentCandidateTraffic(svc, candidate)
	if candidateTarget == nil {
		candidatePercent = r.strategy.Steps[0]
	} else {
		candidatePercent = r.nextCandidateTraffic(candidateTarget.Percent)

		// If the traffic share did not change, candidate already handled 100%
		// and is now ready to become stable.
		if candidatePercent == candidateTarget.Percent {
			promoteToStable = true
		}
	}

	candidateTarget = newTrafficTarget(candidate, candidatePercent, CandidateTag)

	return candidateTarget, promoteToStable
}

// inheritRevisionTags returns the tags that must be conserved.
func inheritRevisionTags(svc *run.Service) []*run.TrafficTarget {
	traffic := []*run.TrafficTarget{
		// Always assign latest tag to the latest revision.
		{LatestRevision: true, Tag: LatestTag},
	}
	// Respect tags manually introduced by the user (e.g. UI/gcloud).
	customTags := userDefinedTrafficTags(svc)
	traffic = append(traffic, customTags...)
	return traffic
}

// userDefinedTrafficTags returns the traffic configurations that include tags
// that were defined by the user (e.g. UI/gcloud).
func userDefinedTrafficTags(svc *run.Service) []*run.TrafficTarget {
	var traffic []*run.TrafficTarget
	for _, target := range svc.Spec.Traffic {
		if target.Tag != "" && !target.LatestRevision &&
			target.Tag != StableTag && target.Tag != CandidateTag {

			traffic = append(traffic, target)
		}
	}

	return traffic
}

// currentCandidateTraffic returns the traffic configuration for the candidate.
func (r *Rollout) currentCandidateTraffic(svc *run.Service, candidate string) *run.TrafficTarget {
	for _, target := range svc.Status.Traffic {
		if target.RevisionName == candidate && target.Percent > 0 {
			return target
		}
	}

	return nil
}

// nextCandidateTraffic calculates the next traffic share for the candidate.
func (r *Rollout) nextCandidateTraffic(current int64) int64 {
	for _, step := range r.strategy.Steps {
		if step > current {
			return step
		}
	}

	return 100
}

// updateAnnotations updates the annotations to keep some state about the rollout.
func (r *Rollout) updateAnnotations(svc *run.Service, stable, candidate string) *run.Service {
	if svc.Metadata.Annotations == nil {
		svc.Metadata.Annotations = make(map[string]string)
	}

	// The candidate has become the stable revision.
	if r.promoteToStable {
		svc.Metadata.Annotations[StableRevisionAnnotation] = candidate
		delete(svc.Metadata.Annotations, CandidateRevisionAnnotation)
		return svc
	}

	svc.Metadata.Annotations[StableRevisionAnnotation] = stable
	svc.Metadata.Annotations[CandidateRevisionAnnotation] = candidate
	if r.shouldRollback {
		svc.Metadata.Annotations[LastFailedCandidateRevisionAnnotation] = candidate
	}

	return svc
}

// diagnoseCandidate returns the candidate's diagnosis based on metrics.
func (r *Rollout) diagnoseCandidate(candidate string, healthCriteria []config.Metric) (d health.Diagnosis, err error) {
	// TODO(gvso): Consider using a different config value for the offset.
	healthCheckOffset := time.Duration(r.strategy.Interval) * time.Second
	r.log.Debug("collecting metrics from API")
	ctx := util.ContextWithLogger(r.ctx, r.log)
	r.metricsProvider.SetCandidateRevision(candidate)
	metricsValues, err := health.CollectMetrics(ctx, r.metricsProvider, healthCheckOffset, healthCriteria)
	if err != nil {
		return d, errors.Wrap(err, "failed to collect metrics")
	}

	r.log.Debug("diagnosing candidate's health")
	d, err = health.Diagnose(ctx, healthCriteria, metricsValues)
	return d, errors.Wrap(err, "failed to diagnose candidate's health")
}

// newTrafficTarget returns a new traffic target instance.
func newTrafficTarget(revision string, percent int64, tag string) *run.TrafficTarget {
	return &run.TrafficTarget{
		RevisionName: revision,
		Percent:      percent,
		Tag:          tag,
	}
}
