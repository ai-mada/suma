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
	case ToolProfile:
		in.Reason = presentationSource(w, in.Reason)
		return in
	case ToolProfileList:
		in.Items = append([]ToolProfile{}, in.Items...)
		for i := range in.Items {
			in.Items[i].Reason = presentationSource(w, in.Items[i].Reason)
		}
		return in
	default:
		return value
	}
}
