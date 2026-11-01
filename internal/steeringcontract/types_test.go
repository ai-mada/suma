package steeringcontract

import (
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

func TestSteeringDomainKeepsTheImmutableHumanSource(t *testing.T) {
	body := " Preserve whitespace.\n继续原任务。 "
	message := SteeringMessage{ID: "message", Sequence: 4, AuthorID: "human", Body: body, BodySHA256: MessageDigest(body)}
	if ValidateMessage(message) != nil {
		t.Fatal("valid original human message rejected")
	}
	long := message
	long.Body = strings.Repeat("码", 8000)
	long.BodySHA256 = MessageDigest(long.Body)
	if ValidateMessage(long) != nil {
		t.Fatal("a valid saved Chinese message was arbitrarily shortened below the discussion producer limit")
	}
	changed := message
	changed.Body = strings.TrimSpace(body)
	if ValidateMessage(changed) == nil {
		t.Fatal("trimmed source accepted with original digest")
	}
	stamp := "2026-11-01T00:00:00Z"
	instruction := SteeringInstruction{ID: "instruction", SessionID: "session", ProjectID: "project", TaskID: "task", RunID: "run", NodeID: "develop", Attempt: 1, Fence: 2, Version: 1, Message: message, ActorID: "human", CreatedAt: stamp, ExpiresAt: stamp}
	if ValidateInstruction(instruction) != nil {
		t.Fatal("valid exact instruction rejected")
	}
	instruction.ActorID = "other"
	if ValidateInstruction(instruction) == nil {
		t.Fatal("another actor became the original human sender")
	}
	session := SteeringSession{ID: "session", ProjectID: "project", TaskID: "task", RunID: "run", NodeID: "develop", Attempt: 1, Fence: 2, WorkerID: "worker", Version: 1, ProfileSHA256: strings.Repeat("a", 64), Source: cc.CodexSteeringSource{Protocol: cc.SteeringProtocol, ThreadID: "thread", TurnID: "turn", BindingSHA256: strings.Repeat("b", 64)}, State: "closed", OpenedAt: stamp, ExpiresAt: stamp, AllowedActions: []string{"send"}}
	if ValidateSession(session) == nil {
		t.Fatal("closed original turn advertised a fresh send")
	}
	session.AllowedActions = []string{}
	if ValidateSession(session) != nil {
		t.Fatal("closed read-only session rejected")
	}
}
