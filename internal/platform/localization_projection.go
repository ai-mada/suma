package platform

import (
	"net/http"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/localization"
)

func presentationSource(w http.ResponseWriter, source string) string {
	key, exists := localization.LookupSource(source)
	if source == "" || !exists {
		return source
	}
	locale, ok := localization.Normalize(w.Header().Get("Content-Language"))
	if !ok {
		locale = localization.DefaultLocale
	}
	translated, err := localization.WithSource(locale, key, nil, source)
	if err != nil || !translated.Localized {
		return source
	}
	return translated.Text
}

func localizeAdmission(w http.ResponseWriter, in dag.NodeAdmissionSnapshot) dag.NodeAdmissionSnapshot {
	out := in
	out.Blockers = append([]dag.SchedulingBlocker{}, in.Blockers...)
	for i := range out.Blockers {
		out.Blockers[i].Message = presentationSource(w, out.Blockers[i].Message)
	}
	return out
}

// Localize only explicit display projections. Never recursively translate
// arbitrary message/name/reason keys: user text, output, saved evidence and
// immutable plan inputs can use those same names. Copy nested slices before
// changing them so serialization cannot mutate an application result/receipt.
func localizePresentation(w http.ResponseWriter, value any) any {
	switch in := value.(type) {
	case RepositoryIntegrationOptions:
		in.Blockers = localizeRepositoryBlockers(w, in.Blockers)
		in.Targets = append([]RepositoryIntegrationTarget{}, in.Targets...)
		for i := range in.Targets {
			in.Targets[i].Blockers = localizeRepositoryBlockers(w, in.Targets[i].Blockers)
		}
		in.Sources = append([]RepositoryIntegrationSourceChoice{}, in.Sources...)
		for i := range in.Sources {
			in.Sources[i].Blockers = localizeRepositoryBlockers(w, in.Sources[i].Blockers)
		}
		return in
	case RepositoryIntegrationPlan:
		return localizeRepositoryPlan(w, in)
	case RepositoryIntegrationTarget:
		in.Blockers = localizeRepositoryBlockers(w, in.Blockers)
		return in
	case RepositoryIntegrationPlanPage:
		in.Items = append([]RepositoryIntegrationPlan{}, in.Items...)
		for i := range in.Items {
			in.Items[i] = localizeRepositoryPlan(w, in.Items[i])
		}
		return in
	case ExecutionPoolPolicyPreview:
		in.Reason = presentationSource(w, in.Reason)
		return in
	case ExecutionRoutePreview:
		in.Reason = presentationSource(w, in.Reason)
		return in
	case SchedulingPolicyPreview:
		in.Reason = presentationSource(w, in.Reason)
		return in
	case TaskDispatchPlan:
		in.Reason = presentationSource(w, in.Reason)
		return in
	case dag.NodeAdmissionSnapshot:
		return localizeAdmission(w, in)
	case dag.ReadyPage:
		in.Items = append([]dag.ReadyEntry{}, in.Items...)
		for i := range in.Items {
			if in.Items[i].Admission != nil {
				copy := localizeAdmission(w, *in.Items[i].Admission)
				in.Items[i].Admission = &copy
			}
		}
		return in
	default:
		return value
	}
}

func localizeRepositoryBlockers(w http.ResponseWriter, values []string) []string {
	out := append([]string{}, values...)
	for i := range out {
		out[i] = presentationSource(w, out[i])
	}
	return out
}
func localizeRepositoryPlan(w http.ResponseWriter, in RepositoryIntegrationPlan) RepositoryIntegrationPlan {
	in.Blockers = localizeRepositoryBlockers(w, in.Blockers)
	in.Target.Blockers = localizeRepositoryBlockers(w, in.Target.Blockers)
	// Do not translate Preview/Observation: their bytes bind approved digests.
	return in
}
