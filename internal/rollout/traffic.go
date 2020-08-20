package rollout

import (
	"time"

	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/health"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/run/v1"
)

// determineTraffic returns a traffic configuration based on the diagnosis.
// If traffic should not changed, nil is returned.
func (r *Rollout) determineTraffic(svc *run.Service, diagnosis health.DiagnosisResult, stable, candidate string) ([]*run.TrafficTarget, bool, error) {
	switch diagnosis {
	case health.Inconclusive:
		r.log.Debug("health check inconclusive")
		return svc.Spec.Traffic, false, nil
	case health.Healthy:
		r.log.Debug("healthy candidate")
		lastRollout := svc.Metadata.Annotations[LastRolloutAnnotation]
		enoughTime, err := r.hasEnoughTimeElapsed(lastRollout, r.strategy.TimeBetweenRollouts)
		if err != nil {
			return nil, false, errors.Wrap(err, "error while determining if enough time elapsed")
		}
		if !enoughTime {
			r.log.WithField("lastRollout", lastRollout).Debug("no enough time elapsed since last roll out")
			return svc.Spec.Traffic, false, nil
		}
		r.log.Info("rolling forward")
		r.shouldRollout = true
		return r.rollForwardTraffic(svc.Spec.Traffic, stable, candidate), true, nil
	case health.Unhealthy:
		r.log.Info("unhealthy candidate, rollback")
		r.shouldRollback = true
		return r.rollbackTraffic(svc.Spec.Traffic, stable, candidate), true, nil
	default:
		return nil, false, errors.Errorf("invalid candidate's health diagnosis %v", diagnosis)
	}
}

// rollForwardTraffic updates the traffic configuration to increase the traffic
// to the candidate.
//
// It creates new traffic configurations for the candidate and stable revisions
// and respects user-defined revision tags.
func (r *Rollout) rollForwardTraffic(traffic []*run.TrafficTarget, stable, candidate string) []*run.TrafficTarget {
	var newTraffic []*run.TrafficTarget
	var stablePercent int64

	candidateTraffic, promoteCandidateToStable := r.newCandidateTraffic(traffic, candidate)
	if promoteCandidateToStable {
		r.promoteToStable = true
		candidateTraffic.Tag = StableTag
	} else {
		// If candidate is not being promoted, also include traffic
		// configuration for stable revision.
		stablePercent = 100 - candidateTraffic.Percent
		stableTraffic := newTrafficTarget(stable, stablePercent, StableTag)
		newTraffic = append(newTraffic, stableTraffic)
	}
	newTraffic = append(newTraffic, candidateTraffic)
	newTraffic = append(newTraffic, inheritRevisionTags(traffic)...)

	if r.promoteToStable {
		r.log.Infof("will make candidate stable")
	} else {
		r.log.WithFields(logrus.Fields{
			"stablePercent":    stablePercent,
			"candidatePercent": candidateTraffic.Percent,
		}).Info("set traffic split")
	}

	return newTraffic
}

// rollbackTraffic redirects all the traffic to the stable revision.
func (r *Rollout) rollbackTraffic(traffic []*run.TrafficTarget, stable, candidate string) []*run.TrafficTarget {
	newTraffic := []*run.TrafficTarget{
		newTrafficTarget(stable, 100, StableTag),
		newTrafficTarget(candidate, 0, CandidateTag),
	}
	return append(newTraffic, inheritRevisionTags(traffic)...)
}

// newCandidateTraffic returns the next candidate's traffic configuration.
//
// It also checks if the candidate should be promoted to stable in the next
// update and returns a boolean about that.
func (r *Rollout) newCandidateTraffic(traffic []*run.TrafficTarget, candidate string) (*run.TrafficTarget, bool) {
	var promoteToStable bool
	var candidatePercent int64
	candidateTarget := r.currentCandidateTraffic(traffic, candidate)
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
func inheritRevisionTags(traffic []*run.TrafficTarget) []*run.TrafficTarget {
	newTraffic := []*run.TrafficTarget{
		// Always assign latest tag to the latest revision.
		{LatestRevision: true, Tag: LatestTag},
	}
	// Respect tags manually introduced by the user (e.g. UI/gcloud).
	customTags := userDefinedTrafficTags(traffic)
	return append(newTraffic, customTags...)
}

// userDefinedTrafficTags returns the traffic configurations that include tags
// that were defined by the user (e.g. UI/gcloud).
func userDefinedTrafficTags(traffic []*run.TrafficTarget) []*run.TrafficTarget {
	var newTraffic []*run.TrafficTarget
	for _, target := range traffic {
		if target.Tag != "" && !target.LatestRevision &&
			target.Tag != StableTag && target.Tag != CandidateTag {

			newTraffic = append(newTraffic, target)
		}
	}

	return newTraffic
}

// currentCandidateTraffic returns the traffic configuration for the candidate.
func (r *Rollout) currentCandidateTraffic(traffic []*run.TrafficTarget, candidate string) *run.TrafficTarget {
	for _, target := range traffic {
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

// newTrafficTarget returns a new traffic target instance.
func newTrafficTarget(revision string, percent int64, tag string) *run.TrafficTarget {
	return &run.TrafficTarget{
		RevisionName: revision,
		Percent:      percent,
		Tag:          tag,
	}
}

// hasEnoughTimeElapsed determines if enough time has elapsed since last
// rollout.
//
// TODO: what if lastRolloutStr is always invalid?
func (r *Rollout) hasEnoughTimeElapsed(lastRolloutStr string, timeBetweenRollouts time.Duration) (bool, error) {
	if lastRolloutStr == "" {
		return false, errors.Errorf("%s annotation is missing", LastRolloutAnnotation)
	}
	lastRollout, err := time.Parse(time.RFC3339, lastRolloutStr)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse last roll out time")
	}

	currentTime := r.time.Now()
	return currentTime.Sub(lastRollout) >= timeBetweenRollouts, nil
}
