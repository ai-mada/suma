//go:build !linux && !windows

package runner

import (
	"fmt"
	"os/exec"
)

func startNativeProcess(c *exec.Cmd, requireContainment bool) (*processController, error) {
	if requireContainment {
		return nil, fmt.Errorf("native probe process containment unavailable")
	}
	return startProcessGroup(c)
}
