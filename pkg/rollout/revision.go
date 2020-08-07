package rollout

import (
	"google.golang.org/api/run/v1"
)

// DetectStableRevisionName returns the stable revision of the Cloud Run service.
//
// It first checks if there's a revision with the tag "stable". If such a
// revision does not exist, it checks for a revision with 100% of the traffic
// and considers it stable.
func DetectStableRevisionName(svc *run.Service) string {
	stableRevision := findRevisionWithTag(svc, StableTag)
	if stableRevision == "" {
		stableRevision = find100PercentServingRevisionName(svc)
		if stableRevision == "" {
			return ""
		}

		return stableRevision
	}

	// In case the stable revision with tag "stable" is not the one handling
	// 100% of the traffic, this recovers from this unexpected situation.
	// This can happen, for instance, if deployment of a revision was done
	// without --no-traffic tag.
	trafficHandler := find100PercentServingRevisionName(svc)
	if trafficHandler != "" && trafficHandler != stableRevision {
		stableRevision = trafficHandler
	}

	return stableRevision
}

// DetectCandidateRevisionName attempts to deduce what revision could be
// considered a candidate.
func DetectCandidateRevisionName(svc *run.Service, stable string) string {
	latestRevision := svc.Status.LatestReadyRevisionName
	if stable == latestRevision {
		return ""
	}

	// If the latestRevision has previously been treated as a candidate and
	// failed to meet health checks, no candidate exists.
	if latestRevision == svc.Metadata.Annotations[LastFailedCandidateRevisionAnnotation] {
		return ""
	}
	return latestRevision
}

// find100PercentServingRevisionName scans the service and retrieves a revision
// with 100% traffic.
func find100PercentServingRevisionName(svc *run.Service) string {
	for _, target := range svc.Status.Traffic {
		if target.Percent == 100 && target.Tag != CandidateTag {
			return target.RevisionName
		}
	}

	return ""
}

// findRevisionWithTag scans the service traffic configuration and returns the
// name of the revision that has the given tag.
func findRevisionWithTag(svc *run.Service, tag string) string {
	for _, target := range svc.Spec.Traffic {
		if target.Tag == tag {
			return target.RevisionName
		}
	}

	return ""
}

// isNewCandidate determines if the candidate was just deployed.
//
// Knowing that a candidate is new is helpful since metrics cannot be obtained
// about it (it has 0 traffic), so the rollout process should add some initial
// traffic to the new revision.
func isNewCandidate(svc *run.Service, currentCandidate string) bool {
	for _, target := range svc.Spec.Traffic {
		if target.RevisionName == currentCandidate {
			return target.Percent == 0
		}
	}

	// Cloud Run (often) removes traffic targets for revisions that have no
	// traffic. If the candidate was not found in the traffic configuration, it
	// means the revision is a new candidate.
	return true
}
