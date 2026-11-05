package runner

import (
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	windowsJobContainment        = "windows_job"
	linuxPIDNamespaceContainment = "linux_pid_namespace"
	unixGroupContainment         = "unix_process_group"
	linuxPIDNamespaceEnv         = "SUMA_RUNNER_LINUX_PID_NAMESPACE"
	processCancellationWait      = 2 * time.Second
)

func startProcess(c *exec.Cmd) (*processController, error) {
	return startNativeProcess(c, false)
}

func authoritativeProcessContainment(containment string) bool {
	return containment == windowsJobContainment || containment == linuxPIDNamespaceContainment
}

func nativeProbeContainmentConfigured() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "linux" && os.Getenv(linuxPIDNamespaceEnv) == "1"
}

func waitForNativeCompletion[T any](done <-chan T, timeout time.Duration) (T, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-done:
		return result, true
	case <-timer.C:
		var zero T
		return zero, false
	}
}
