package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestCodexAppServerCancelBeforeQueuedTurnWriteSubmitsNothing(t *testing.T) {
	d, _, _ := codexSkillsDriver(t, false)
	defer d.Close()
	// The native thread ACK has queued the real prompt, but the stdin writer
	// has not claimed it. Cancellation must prevent even the first write.
	codexSkillsThreadACK(t, d)
	if d.bad != nil || d.waiting != "turn/start" || d.turn != "" || !strings.Contains(string(d.input.pending), `"method":"turn/start"`) {
		t.Fatalf("fixture did not queue an unclaimed turn: %s / %v", d.input.pending, d.bad)
	}
	if err := d.Cancel(); err != nil {
		t.Fatal(err)
	}
	var written bytes.Buffer
	n, err := d.CopyInput(&written)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || written.Len() != 0 {
		t.Fatalf("cancelled attempt wrote an unclaimed prompt: %s", written.Bytes())
	}
}
