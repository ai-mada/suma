package runner

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

// sealCodexDevelopment commits the stopped agent's edits in the attempt worktree.
// The Runner owns Git metadata. The agent uses its original workspace sandbox.
// Candidate transfer, independent testing and Review retain their normal gates.
func sealCodexDevelopment(ctx context.Context, w *attemptWorkspace, binding cc.Binding, treeStopped bool, result *Result) {
	if result.ExitCode != 0 || result.Error != "" || result.Stopped || result.OutcomeUnknown {
		return
	}
	fail := func(message string) {
		result.ExitCode = -1
		result.Error = message
	}
	if !treeStopped || result.ToolEvidence == nil || result.ToolEvidence.Terminal != "completed" || result.ToolEvidence.CodexExecution == nil || !cc.EvidenceMatchesBinding(binding, *result.ToolEvidence.CodexExecution) {
		fail("Codex candidate requires a confirmed execution and process stop")
		result.OutcomeUnknown = true
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if _, err := nativeSealIdentity(ctx, w); err != nil {
		fail(err.Error())
		return
	}
	head, err := nativeSealText(ctx, w, "", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		fail("Codex candidate worktree HEAD is unavailable")
		return
	}
	if head != w.BaseCommit {
		// A committed candidate remains on the original clean-HEAD transfer path.
		return
	}
	inventory, _, err := stoppedWorkspaceInventory(ctx, w)
	if err != nil {
		fail(err.Error())
		return
	}
	if len(inventory.Files) == 0 {
		return
	}
	if err := checkCodexCandidateFiles(ctx, w, inventory.Files); err != nil {
		fail(err.Error())
		return
	}
	evidence := terminalCandidateNativeEvidence(inventory, result.ToolEvidence.Profile.ProfileSHA256)
	seal, err := sealNativeWorkspace(ctx, w, evidence)
	if err != nil || seal.State != "sealed" {
		fail("Codex candidate could not be sealed from the stopped worktree")
		result.OutcomeUnknown = seal.State == "prepared" || seal.State == "unknown"
	}
}

// The parent seal writes Git objects directly. Require Git's normal untracked
// admission first, with the attempt's baseline ignore files still in place.
// Tracked modifications and removals remain subject to the parent seal checks.
func checkCodexCandidateFiles(ctx context.Context, w *attemptWorkspace, effects []NativeWorkspaceFileEffect) error {
	newPaths := make(map[string]bool)
	for _, effect := range effects {
		if strings.EqualFold(path.Base(effect.Path), ".gitignore") {
			return fmt.Errorf("Codex candidate changed Git ignore rules and requires review")
		}
		if !effect.Removed && effect.PreviousSHA256 == "" {
			newPaths[effect.Path] = true
		}
	}
	if len(newPaths) == 0 {
		return nil
	}
	allowed, err := nativeSealGit(ctx, w, "", nil, 4<<20, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return fmt.Errorf("Codex candidate could not confirm Git ignore rules")
	}
	for _, file := range strings.Split(string(allowed), "\x00") {
		delete(newPaths, file)
	}
	if len(newPaths) != 0 {
		return fmt.Errorf("Codex candidate contains ignored or staged new files and requires review")
	}
	return nil
}
