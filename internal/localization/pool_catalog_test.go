package localization

import (
	"strings"
	"testing"
)

func TestPoolCatalogPreservesSpecificCapacityAndAuthorityMeaning(t *testing.T) {
	for _, example := range []struct{ source, key, fragment string }{
		{"共享池配置准入资源不足；不是已测硬件负载", "pool.admission.resource_capacity", "not a measurement of hardware load"},
		{"共享池累计准入单位不足；不代表金额或token账单", "pool.admission.budget_exhausted", "not monetary charges or token bills"},
		{"请先排空正在执行的任务并核对失联结果，再轮换凭证", "pool.runner.rotation_requires_drain", "reconcile disconnected attempts"},
	} {
		key, ok := LookupSource(example.source)
		if !ok || key != example.key {
			t.Fatalf("missing exact source mapping for %s", example.key)
		}
		rendered, err := WithSource(EnUS, key, nil, example.source)
		if err != nil || !rendered.Localized || !strings.Contains(rendered.Text, example.fragment) {
			t.Fatal(rendered, err)
		}
	}
}
