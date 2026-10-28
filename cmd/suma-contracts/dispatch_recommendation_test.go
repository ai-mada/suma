package main

import "testing"

func TestDispatchRecommendationContractsKeepReadAnalysisAndHumanAdoptionDistinct(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	schemas := mapAt(api, "components", "schemas")
	if mapAt(paths, "/dispatch-recommendations/options", "post")["x-read-only"] != true {
		t.Fatal("candidate options acquired a side effect")
	}
	if mapAt(paths, "/dispatch-recommendations/{id}/adopt", "post")["x-read-only"] == true {
		t.Fatal("human plan adoption was marked as a read")
	}
	choice := mapAt(schemas, "DispatchRecommendationChoice")
	if choice["additionalProperties"] != false || len(mapAt(choice, "properties")) != 5 {
		t.Fatal("choice contract gained undeclared dispatch fields")
	}
	if mapAt(schemas, "DispatchRecommendationSource", "properties", "candidates")["maxItems"] != 24 {
		t.Fatal("model context is unbounded")
	}
	if mapAt(schemas, "DispatchRecommendationProvenance", "properties", "choice_sha256")["readOnly"] != true {
		t.Fatal("public input can manufacture adoption proof")
	}
}
