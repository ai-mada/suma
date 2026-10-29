//go:build windows

package terminalruntime

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	pty "github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/windows"
)

// Windows starts suspended before user code can create an uncontained child.
// The job forbids breakaway and kills all remaining descendants on close.
type windowsTree struct {
	mu  sync.Mutex
	job windows.Handle
}

const BootstrapFlag = "--suma-terminal-pty-child"

func newPTYCommand(p pty.Pty, c Config) (*pty.Cmd, error) {
	bootstrap := c.BootstrapExecutable
	if bootstrap == "" {
		var err error
		bootstrap, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	if !filepath.IsAbs(bootstrap) {
		return nil, ErrInvalidConfig
	}
	args := append([]string{}, c.BootstrapArgs...)
	args = append(args, BootstrapFlag, "--", c.Executable)
	args = append(args, c.Args...)
	return p.Command(bootstrap, args...), nil
}

// HandleChildProcess is opt-in at the Runner/embedded-control executable
// entrypoint. It never changes the long-lived Runner's console signal policy.
// Windows can inherit IGNORE_CTRLC through a GUI/service launch chain; clearing
// it only in this dedicated already-contained child lets the real shell and
// its foreground descendants receive ConPTY ETX normally.
func HandleChildProcess(args []string) (bool, int) {
	if len(args) == 0 || args[0] != BootstrapFlag {
		return false, 0
	}
	fail := func(message string) (bool, int) { _, _ = fmt.Fprintln(os.Stderr, message); return true, 125 }
	if len(args) < 3 || args[1] != "--" || !filepath.IsAbs(args[2]) || len(args) > 130 {
		return fail("Terminal child bootstrap arguments are invalid")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return fail("Terminal child bootstrap arguments are invalid")
		}
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	result, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler").Call(0, 0)
	if result == 0 {
		return fail(fmt.Sprintf("Terminal child could not enable console interrupts: %v", err))
	}
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return fail("Terminal child console input is unavailable")
	}
	defer in.Close()
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		return fail("Terminal child console output is unavailable")
	}
	defer out.Close()
	var mode uint32
	if err = windows.GetConsoleMode(windows.Handle(in.Fd()), &mode); err != nil {
		return fail("Terminal child console input mode is unavailable")
	}
	if err = windows.SetConsoleMode(windows.Handle(in.Fd()), mode|windows.ENABLE_PROCESSED_INPUT); err != nil {
		return fail("Terminal child could not enable processed input")
	}
	command := exec.Command(args[2], args[3:]...)
	command.Stdin = in
	command.Stdout = out
	command.Stderr = out
	// This process already has an explicit isolated environment and current
	// working directory. A child inherits those, including enabled Ctrl-C.
	command.Env = os.Environ()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return true, exit.ExitCode()
		}
		return fail("Terminal child shell could not start")
	}
	return true, 0
}

func startPTYProcess(c *pty.Cmd) (treeController, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	tree := &windowsTree{job: job}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		tree.Close()
		return nil, err
	}
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED, HideWindow: true}
	if err = c.Start(); err != nil {
		tree.Close()
		return nil, err
	}
	fail := func(cause error) (treeController, error) {
		_ = c.Process.Kill()
		_ = c.Wait()
		tree.Close()
		return nil, cause
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if err != nil {
		return fail(err)
	}
	err = windows.AssignProcessToJobObject(job, process)
	_ = windows.CloseHandle(process)
	if err != nil {
		return fail(fmt.Errorf("terminal process could not enter its Windows job: %w", err))
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(c.Process.Pid) {
			continue
		}
		thread, e := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if e != nil {
			return fail(e)
		}
		previous, e := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if e != nil || previous != 1 {
			return fail(fmt.Errorf("terminal suspended initial thread could not resume: %v", e))
		}
		return tree, nil
	}
	return fail(fmt.Errorf("terminal suspended initial thread was not found"))
}

func (t *windowsTree) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 {
		return ErrTreeUnknown
	}
	if err := windows.TerminateJobObject(t.job, 1); err != nil {
		return err
	}
	// Job basic accounting is stable on all supported ConPTY Windows targets.
	type accounting struct {
		Times                                 [4]int64
		PageFaults, Total, Active, Terminated uint32
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var info accounting
		if err := windows.QueryInformationJobObject(t.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
			return err
		}
		if info.Active == 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ErrTreeUnknown
}

func (t *windowsTree) Finish() error { return t.Stop() }
func (t *windowsTree) ActiveProcesses() (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 {
		return 0, ErrTreeUnknown
	}
	type accounting struct {
		Times                                 [4]int64
		PageFaults, Total, Active, Terminated uint32
	}
	var info accounting
	if err := windows.QueryInformationJobObject(t.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return 0, err
	}
	return int(info.Active), nil
}
func (t *windowsTree) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		_ = windows.CloseHandle(t.job)
		t.job = 0
	}
}
