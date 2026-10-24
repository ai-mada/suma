package platform

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
)

var executionPoolID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func validateExecutionPoolPolicy(input ExecutionPoolPolicyInput) error {
	if err := validateSchedulingPolicy(dag.SchedulingPolicyInput{ConcurrencyLimit: input.ConcurrencyLimit, ResourceLimits: input.ResourceLimits, BudgetLimit: input.BudgetLimit}); err != nil {
		return err
	}
	if len(input.TenantWeights) > 256 {
		return invalid("每个资源池最多配置 256 个组织权重")
	}
	for tenant, weight := range input.TenantWeights {
		if !executionPoolID.MatchString(tenant) || weight < 1 || weight > 100 {
			return invalid("组织标识或权重无效；权重必须为 1–100 的整数")
		}
	}
	return nil
}

func validatePoolOperation(value string) error {
	if len(value) < 8 || len(value) > 128 || !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return invalid("操作标识必须为 8–128 字节的有效文本")
	}
	return nil
}

func validatePoolName(name string) error {
	if strings.TrimSpace(name) != name || name == "" || !ingressText(name, 200) {
		return invalid("名称必须为 1–200 字节的有效文本")
	}
	return nil
}

func validatePoolTTL(ttl int) error {
	if ttl < 3600 || ttl > 30*86400 {
		return invalid("执行节点凭证有效期必须为 1 小时至 30 天")
	}
	return nil
}

func validateExecutionPoolRunner(request ExecutionPoolRunnerCreateRequest) error {
	if err := validatePoolName(request.Name); err != nil {
		return err
	}
	if request.MaxSlots < 1 || request.MaxSlots > 128 {
		return invalid("执行节点并发槽位必须为 1–128 的整数")
	}
	if err := validatePoolTTL(request.TTLSeconds); err != nil {
		return err
	}
	return validatePoolOperation(request.IdempotencyKey)
}
