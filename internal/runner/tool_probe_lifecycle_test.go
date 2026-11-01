package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestToolProbeLifecycleHelper195(t *testing.T) {
	mode := os.Getenv("SUMA_PROBE195_MODE")
	if mode == "" {
		return
	}
	if path := os.Getenv("SUMA_PROBE195_STARTED"); path != "" {
		_ = os.WriteFile(path, []byte("owned probe started"), 0600)
	}
	if mode == "cancel" {
		time.Sleep(30 * time.Second)
	}
	if mode == "nonzero" {
		os.Exit(7)
	}
	os.Exit(0)
}

func TestProbeLifecyclePersistsEveryExitAndTimeoutWithoutInventedStop(t *testing.T) {
	for _, mode := range []string{"normal", "nonzero", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			j, err := OpenJournal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			client := &Client{BaseURL: "http://127.0.0.1:19500", ProjectID: "project195", Token: "fixture195"}
			cp, err := j.beginToolInspection(client, "worker195")
			if err != nil {
				t.Fatal(err)
			}
			i := inspectionFixture194(client.ProjectID, "worker195", cp.ClaimID)
			cp.Responded = true
			cp.Inspection = &i
			cp.Started = true
			if err = j.saveToolInspection(*cp); err != nil {
				t.Fatal(err)
			}
			collector, err := newToolProbeCollector(j, *cp)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(collector.context(context.Background()))
			defer cancel()
			started := filepath.Join(t.TempDir(), "started")
			if mode == "cancel" {
				go func() {
					deadline := time.Now().Add(2 * time.Second)
					for time.Now().Before(deadline) {
						if _, e := os.Stat(started); e == nil {
							cancel()
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
					cancel()
				}()
			}
			exe, _ := os.Executable()
			_, probeErr := probeCommand(ctx, exe, []string{"-test.run=^TestToolProbeLifecycleHelper195$"}, CommandSpec{Env: map[string]string{"SUMA_PROBE195_MODE": mode, "SUMA_PROBE195_STARTED": started}}, nil)
			if (probeErr == nil) != (mode == "normal") {
				t.Fatal("wrong native outcome", probeErr)
			}
			if err = collector.closeReturned(); err != nil {
				t.Fatal(err)
			}
			receipt, sha, err := j.readToolProbeReceipt(*cp)
			if err != nil {
				t.Fatal(err)
			}
			if !receipt.ProbeReturned || receipt.NativeProcessCount != 1 || receipt.StoppedProcessCount != func() int {
				if runtime.GOOS == "windows" {
					return 1
				}
				return 0
			}() || receipt.AllNativeTreesStopped != (runtime.GOOS == "windows") || len(sha) != 64 {
				t.Fatal("actual process stop facts were lost or invented", receipt)
			}
			// Reopening reads the same immutable original receipt, never starts
			// another native process and never consults a stored PID for ownership.
			_ = j.Close()
			j, err = OpenJournal(filepath.Dir(j.path))
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			_, again, err := j.readToolProbeReceipt(*cp)
			if err != nil || again != sha {
				t.Fatal("durable original stop receipt changed", err)
			}
		})
	}
}
