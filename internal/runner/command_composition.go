package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-mada/suma/internal/strictjson"
)

const CommandCompositionProtocol = "suma-command-composition-v1"
const commandCompositionInputLimit = 64 << 10
const commandCompositionFileLimit = 2 << 20

// CommandComposition identifies only the new local configuration bytes. It is
// neither an installation observation nor permission to execute these commands.
type CommandComposition struct {
	Protocol            string `json:"protocol"`
	SourceCount         int    `json:"source_count"`
	CommandCount        int    `json:"command_count"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
}

// Only closed diagnostic facts cross into the CLI. Never wrap a filesystem,
// decoder or command-policy error: those can contain private paths or values.
type CommandCompositionError struct {
	Code        string
	SourceIndex int
}

func (*CommandCompositionError) Error() string {
	return "local command composition failed; preserve original files"
}

func commandCompositionFailure(code string, source int) error {
	return &CommandCompositionError{Code: code, SourceIndex: source}
}

func commandCompositionPath(path string) bool {
	// The existing preparation predicate handles native OS aliases and byte
	// bounds. This mode always names a file, so reject a trailing separator too.
	return poolPreparationPath(path) && !strings.HasSuffix(path, string(filepath.Separator)) && !(filepath.Separator == '\\' && strings.HasSuffix(path, "/"))
}

// Re-read the exact original files, not a cached normalized key. Comparing
// identity and bytes also catches same-size, timestamp-preserving replacement.
// Rechecking every ancestor keeps a renamed/replaced parent from redirecting
// the final operation to another source or output directory.
func validateCommandCompositionSources(ctx context.Context, sources []poolPreparationFile) error {
	for index, source := range sources {
		if ctx.Err() != nil {
			return commandCompositionFailure("cancelled", 0)
		}
		if !cursorPackageParentsStable(source.parents) {
			return commandCompositionFailure("inputs_changed", index+1)
		}
		current, err := inspectPoolPreparationFile(source.path, commandCompositionFileLimit)
		if err != nil || !os.SameFile(source.info, current.info) || !bytes.Equal(source.raw, current.raw) {
			return commandCompositionFailure("inputs_changed", index+1)
		}
	}
	return nil
}

// PrepareCommandComposition combines explicit, existing administrator files.
// It does not resolve executables, read credentials, contact the control plane,
// probe a runtime, register a tool or run a command. Source fragments are never
// individually treated as complete executable configuration.
func PrepareCommandComposition(ctx context.Context, destination string, input io.Reader) (CommandComposition, error) {
	var report CommandComposition
	if !commandCompositionPath(destination) {
		return report, commandCompositionFailure("output_invalid", 0)
	}
	// Refuse an existing target before reading private stdin. The original
	// rooted writer will independently repeat O_EXCL at the actual write.
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err == nil {
			return report, commandCompositionFailure("output_exists", 0)
		}
		return report, commandCompositionFailure("output_invalid", 0)
	}
	file, parent, err := openToolCommandDestination(destination)
	if err != nil {
		if _, existsErr := os.Lstat(destination); existsErr == nil {
			return report, commandCompositionFailure("output_exists", 0)
		}
		return report, commandCompositionFailure("output_invalid", 0)
	}
	defer parent.Close()
	_, parents, err := cursorPackageRoot(filepath.Dir(file))
	if err != nil {
		return report, commandCompositionFailure("output_invalid", 0)
	}
	anchored, err := parent.Stat(".")
	if err != nil || !os.SameFile(anchored, parents[0].info) {
		return report, commandCompositionFailure("output_invalid", 0)
	}
	if ctx.Err() != nil {
		return report, commandCompositionFailure("cancelled", 0)
	}
	if input == nil {
		return report, commandCompositionFailure("input_invalid", 0)
	}
	raw, err := io.ReadAll(io.LimitReader(input, commandCompositionInputLimit+1))
	if ctx.Err() != nil {
		return report, commandCompositionFailure("cancelled", 0)
	}
	var selected struct {
		Files []string `json:"files"`
	}
	if err != nil || strictjson.Decode(raw, &selected, commandCompositionInputLimit) != nil || len(selected.Files) < 2 || len(selected.Files) > 16 {
		return report, commandCompositionFailure("input_invalid", 0)
	}
	sources := make([]poolPreparationFile, 0, len(selected.Files))
	commands := map[string]CommandSpec{}
	readBytes := 0
	for index, path := range selected.Files {
		if ctx.Err() != nil {
			return report, commandCompositionFailure("cancelled", 0)
		}
		if !commandCompositionPath(path) {
			return report, commandCompositionFailure("source_unavailable", index+1)
		}
		source, readErr := inspectPoolPreparationFile(path, commandCompositionFileLimit-readBytes)
		if ctx.Err() != nil {
			return report, commandCompositionFailure("cancelled", 0)
		}
		if readErr != nil {
			return report, commandCompositionFailure("source_unavailable", index+1)
		}
		readBytes += len(source.raw)
		for _, prior := range sources {
			// Do not silently deduplicate repeated paths, differently cased
			// spellings or hard links to the same administrator source file.
			if os.SameFile(prior.info, source.info) {
				return report, commandCompositionFailure("source_invalid", index+1)
			}
		}
		fragment, decodeErr := decodeCommandProfiles(source.raw)
		if decodeErr != nil || len(fragment) == 0 {
			return report, commandCompositionFailure("source_invalid", index+1)
		}
		for id, spec := range fragment {
			if _, duplicate := commands[id]; duplicate {
				return report, commandCompositionFailure("duplicate_command", index+1)
			}
			commands[id] = spec
		}
		if len(commands) > 128 {
			return report, commandCompositionFailure("configuration_invalid", 0)
		}
		sources = append(sources, source)
	}
	encoded, err := json.MarshalIndent(commands, "", "  ")
	if err != nil || len(encoded)+1 > commandCompositionFileLimit {
		return report, commandCompositionFailure("configuration_invalid", 0)
	}
	encoded = append(encoded, '\n')
	// This is the unchanged whole-configuration authority: checks may now
	// resolve across sources, but no protocol, model or test policy is added.
	if _, err = DecodeCommands(encoded); err != nil {
		return report, commandCompositionFailure("configuration_invalid", 0)
	}
	if err = validateCommandCompositionSources(ctx, sources); err != nil {
		return report, err
	}
	if !cursorPackageParentsStable(parents) {
		return report, commandCompositionFailure("inputs_changed", 0)
	}
	if ctx.Err() != nil {
		return report, commandCompositionFailure("cancelled", 0)
	}
	// Reuse the mature rooted exclusive-create, sync and close path. Its final
	// validation is intentional: this preparation mode introduces no new writer
	// or weaker route around the existing complete configuration validator.
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), commands); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return report, commandCompositionFailure("cancelled", 0)
		}
		return report, commandCompositionFailure("output_unconfirmed", 0)
	}
	// Once creation was attempted, failed observation is unknown output, never
	// permission to remove/overwrite/retry it. Leave any new file for inspection.
	confirmed, err := inspectPoolPreparationFile(file, commandCompositionFileLimit)
	if err != nil || !cursorPackageParentsStable(parents) || !bytes.Equal(confirmed.raw, encoded) {
		return report, commandCompositionFailure("output_unconfirmed", 0)
	}
	return CommandComposition{Protocol: CommandCompositionProtocol, SourceCount: len(sources), CommandCount: len(commands), ConfigurationSHA256: digest(encoded)}, nil
}
