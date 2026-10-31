package platform

// This is a model-text projection, never a replacement source or proof digest.
// Complete bases remain in immutable sources and the authoritative TaskContext.
type knowledgeDeliveryPromptSummary struct {
	BasisSHA256    string `json:"basis_sha256"`
	TestCount      int    `json:"test_count"`
	CandidateCount int    `json:"candidate_count"`
}

func knowledgeDeliverySummary(b *DeliveryKnowledgeBasis) *knowledgeDeliveryPromptSummary {
	if b == nil {
		return nil
	}
	return &knowledgeDeliveryPromptSummary{BasisSHA256: b.SHA256, TestCount: len(b.Tests), CandidateCount: len(b.Candidates)}
}

type knowledgeModelSource struct {
	Kind            string                          `json:"kind"`
	ID              string                          `json:"id"`
	Version         int                             `json:"version"`
	Title           string                          `json:"title"`
	Quote           string                          `json:"quote"`
	SHA256          string                          `json:"sha256"`
	DeliverySummary *knowledgeDeliveryPromptSummary `json:"delivery_summary,omitempty"`
}

func knowledgeIntelligencePromptSource(source KnowledgeIntelligenceSource) any {
	hasDelivery := false
	for _, s := range source.Sources {
		hasDelivery = hasDelivery || s.Delivery != nil
	}
	if !hasDelivery {
		return source
	}
	items := make([]knowledgeModelSource, len(source.Sources))
	for i, s := range source.Sources {
		items[i] = knowledgeModelSource{Kind: s.Kind, ID: s.ID, Version: s.Version, Title: s.Title, Quote: s.Quote, SHA256: s.SHA256, DeliverySummary: knowledgeDeliverySummary(s.Delivery)}
	}
	return struct {
		SHA256  string                 `json:"sha256"`
		Sources []knowledgeModelSource `json:"sources"`
	}{source.SHA256, items}
}

type knowledgeModelContextSource struct {
	Kind            string                          `json:"kind"`
	ID              string                          `json:"id"`
	Version         int                             `json:"version"`
	Title           string                          `json:"title"`
	SHA256          string                          `json:"sha256"`
	DeliverySummary *knowledgeDeliveryPromptSummary `json:"delivery_summary,omitempty"`
}
type knowledgeModelContextItem struct {
	ProjectKnowledgeContextItem
	Sources []knowledgeModelContextSource `json:"sources"`
}

func taskKnowledgePromptContext(task TaskContext) any {
	hasDelivery := false
	for _, item := range task.Knowledge {
		for _, s := range item.Sources {
			hasDelivery = hasDelivery || s.Delivery != nil
		}
	}
	if !hasDelivery {
		return task
	}
	items := make([]knowledgeModelContextItem, len(task.Knowledge))
	for i, item := range task.Knowledge {
		var sources []knowledgeModelContextSource
		if item.Sources != nil {
			sources = make([]knowledgeModelContextSource, len(item.Sources))
		}
		for j, s := range item.Sources {
			sources[j] = knowledgeModelContextSource{Kind: s.Kind, ID: s.ID, Version: s.Version, Title: s.Title, SHA256: s.SHA256, DeliverySummary: knowledgeDeliverySummary(s.Delivery)}
		}
		items[i] = knowledgeModelContextItem{ProjectKnowledgeContextItem: item, Sources: sources}
	}
	return struct {
		TaskContext
		Knowledge []knowledgeModelContextItem `json:"knowledge,omitempty"`
	}{task, items}
}
