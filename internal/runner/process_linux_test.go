package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

const linuxNamespaceFixture = "-test.run=^TestLinuxPIDNamespaceFixture$"

func TestLinuxPIDNamespaceStopsDetachedDescendants(t *testing.T) {
	if os.Getenv(linuxPIDNamespaceEnv) != "1" {
		t.Skip("requires a host explicitly enabled for Linux PID namespaces")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"finish", "stop"} {
		t.Run(action, func(t *testing.T) {
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			defer write.Close()
			release := filepath.Join(t.TempDir(), "exit")
			cmd := exec.Command(exe, linuxNamespaceFixture, "--", "init", release, strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Getegid()))
			cmd.ExtraFiles = []*os.File{write}
			controller, err := startProcess(cmd)
			if err != nil {
				t.Fatal(err)
			}
			defer controller.Close()
			_ = write.Close()
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			if controller.containment() != linuxPIDNamespaceContainment {
				t.Fatal("unexpected native process backend", controller.containment())
			}
			var ready [2]byte
			if err = read.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err = io.ReadFull(read, ready[:]); err != nil || (string(ready[:]) != "RP" && string(ready[:]) != "PR") {
				t.Fatal("detached fixture did not start", err)
			}
			if action == "stop" {
				if err = controller.Stop(); err != nil {
					t.Fatal(err)
				}
			} else if err = os.WriteFile(release, nil, 0600); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if (err == nil) != (action == "finish") {
					t.Fatal("wrong original process exit", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("original process did not exit")
			}
			if err = controller.Finish(); err != nil {
				t.Fatal(err)
			}
			if err = read.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err = read.Read(ready[:]); !errors.Is(err, io.EOF) {
				t.Fatal("detached descendant still held the original pipe", err)
			}
		})
	}
}

func TestLinuxPIDNamespaceFixture(t *testing.T) {
	if len(os.Args) != 7 || os.Args[1] != linuxNamespaceFixture || os.Args[2] != "--" {
		return
	}
	pipe := os.NewFile(3, "fixture-ready")
	defer pipe.Close()
	role, release := os.Args[3], os.Args[4]
	if role == "detached" {
		if _, err := pipe.Write([]byte{'R'}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Second)
		return
	}
	if role == "init" {
		if os.Getpid() != 1 || strconv.Itoa(os.Geteuid()) != os.Args[5] || strconv.Itoa(os.Getegid()) != os.Args[6] {
			t.Fatal("namespace fixture did not preserve original identity")
		}
		fork := exec.Command(os.Args[0], linuxNamespaceFixture, "--", "fork", release, os.Args[5], os.Args[6])
		fork.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		fork.ExtraFiles = []*os.File{pipe}
		if err := fork.Run(); err != nil {
			t.Fatal(err)
		}
		if _, err := pipe.Write([]byte{'P'}); err != nil {
			t.Fatal(err)
		}
		_ = pipe.Close()
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			if _, err := os.Stat(release); err == nil {
				return
			}
		}
		t.Fatal("fixture did not receive its exit signal")
	}
	fork := exec.Command(os.Args[0], linuxNamespaceFixture, "--", "detached", release, os.Args[5], os.Args[6])
	fork.ExtraFiles = []*os.File{pipe}
	if err := fork.Start(); err != nil {
		t.Fatal(err)
	}
	_ = fork.Process.Release()
}
