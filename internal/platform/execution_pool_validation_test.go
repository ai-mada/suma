package platform

import (
	"strings"
	"testing"
)

func TestExecutionPoolPolicyAdmissionBoundaries(t *testing.T) {
	zero, one := int64(0), int64(1)
	valid := ExecutionPoolPolicyInput{ConcurrencyLimit: 0, ResourceLimits: map[string]*int64{"cpu": nil, "gpu": &zero}, BudgetLimit: &zero, TenantWeights: map[string]int{strings.Repeat("a", 32): 1}}
	if err := validateExecutionPoolPolicy(valid); err != nil {
		t.Fatal("Explicit zero and unknown/unlimited must survive validation:", err)
	}
	if valid.ResourceLimits["cpu"] != nil || *valid.BudgetLimit != 0 || *valid.ResourceLimits["gpu"] != 0 {
		t.Fatal("validation changed zero/unlimited semantics")
	}
	for _, tc := range []struct {
		name  string
		input ExecutionPoolPolicyInput
	}{
		{"negative slots", ExecutionPoolPolicyInput{ConcurrencyLimit: -1}},
		{"unsafe slots", ExecutionPoolPolicyInput{ConcurrencyLimit: 9007199254740992}},
		{"reserved slots", ExecutionPoolPolicyInput{ResourceLimits: map[string]*int64{"execution_slots": &one}}},
		{"reserved budget", ExecutionPoolPolicyInput{ResourceLimits: map[string]*int64{"budget": &one}}},
		{"zero weight", ExecutionPoolPolicyInput{TenantWeights: map[string]int{strings.Repeat("a", 32): 0}}},
		{"excess weight", ExecutionPoolPolicyInput{TenantWeights: map[string]int{strings.Repeat("a", 32): 101}}},
		{"invalid tenant", ExecutionPoolPolicyInput{TenantWeights: map[string]int{"../another": 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateExecutionPoolPolicy(tc.input); err == nil {
				t.Fatal("unsafe admission input accepted")
			}
		})
	}
}

func TestExecutionPoolRunnerRegistrationIsBoundedAndNamed(t *testing.T) {
	valid := ExecutionPoolRunnerCreateRequest{Name: "共享开发节点", MaxSlots: 4, TTLSeconds: 86400, IdempotencyKey: "register-operation-1"}
	if err := validateExecutionPoolRunner(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ExecutionPoolRunnerCreateRequest){
		func(r *ExecutionPoolRunnerCreateRequest) { r.MaxSlots = 0 },
		func(r *ExecutionPoolRunnerCreateRequest) { r.MaxSlots = 129 },
		func(r *ExecutionPoolRunnerCreateRequest) { r.TTLSeconds = 3599 },
		func(r *ExecutionPoolRunnerCreateRequest) { r.TTLSeconds = 30*86400 + 1 },
		func(r *ExecutionPoolRunnerCreateRequest) { r.Name = " " },
		func(r *ExecutionPoolRunnerCreateRequest) { r.Name = strings.Repeat("x", 201) },
		func(r *ExecutionPoolRunnerCreateRequest) { r.Name = string([]byte{0xff}) },
		func(r *ExecutionPoolRunnerCreateRequest) { r.IdempotencyKey = "new" },
	} {
		input := valid
		mutate(&input)
		if err := validateExecutionPoolRunner(input); err == nil {
			t.Fatalf("unsafe registration accepted: %+v", input)
		}
	}
}
