//go:build windows

package runner

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")

type processController struct{ job syscall.Handle }
type jobLimits struct {
	PerProcessTime, PerJobTime int64
	Flags                      uint32
	Min, Max                   uintptr
	Active                     uint32
	Affinity                   uintptr
	Priority, Scheduling       uint32
}
type jobExtended struct {
	Basic                                          jobLimits
	IO                                             [6]uint64
	ProcessMemory, JobMemory, PeakProcess, PeakJob uintptr
}
type threadEntry struct {
	Size, Usage, ID, Owner uint32
	Base, Delta            int32
	Flags                  uint32
}

func startNativeProcess(c *exec.Cmd, _ bool) (*processController, error) {
	job, _, e := kernel.NewProc("CreateJobObjectW").Call(0, 0)
	if job == 0 {
		return nil, e
	}
	p := &processController{syscall.Handle(job)}
	limits := jobExtended{}
	limits.Basic.Flags = 0x2000
	ok, _, e := kernel.NewProc("SetInformationJobObject").Call(job, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if ok == 0 {
		p.Close()
		return nil, e
	}
	// Suspend before user code executes, attach to the job, then resume the initial thread.
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x4, HideWindow: true}
	if err := c.Start(); err != nil {
		p.Close()
		return nil, err
	}
	fail := func(err error) (*processController, error) {
		_ = c.Process.Kill()
		_ = c.Wait()
		p.Close()
		return nil, err
	}
	process, _, e := kernel.NewProc("OpenProcess").Call(0x0100|0x0001, 0, uintptr(c.Process.Pid))
	if process == 0 {
		return fail(e)
	}
	ok, _, e = kernel.NewProc("AssignProcessToJobObject").Call(job, process)
	syscall.CloseHandle(syscall.Handle(process))
	if ok == 0 {
		return fail(fmt.Errorf("cannot isolate process in Windows job: %w", e))
	}
	snapshot, _, e := kernel.NewProc("CreateToolhelp32Snapshot").Call(0x4, 0)
	if snapshot == ^uintptr(0) {
		return fail(e)
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))
	te := threadEntry{Size: uint32(unsafe.Sizeof(threadEntry{}))}
	ok, _, _ = kernel.NewProc("Thread32First").Call(snapshot, uintptr(unsafe.Pointer(&te)))
	for ok != 0 {
		if te.Owner == uint32(c.Process.Pid) {
			h, _, err := kernel.NewProc("OpenThread").Call(0x2, 0, uintptr(te.ID))
			if h == 0 {
				return fail(err)
			}
			n, _, err := kernel.NewProc("ResumeThread").Call(h)
			syscall.CloseHandle(syscall.Handle(h))
			if n == ^uintptr(0) || uint32(n) == 0xffffffff {
				return fail(err)
			}
			return p, nil
		}
		ok, _, _ = kernel.NewProc("Thread32Next").Call(snapshot, uintptr(unsafe.Pointer(&te)))
	}
	return fail(fmt.Errorf("suspended process initial thread not found"))
}

func (p *processController) containment() string { return windowsJobContainment }

func (p *processController) Stop() error {
	r, _, e := kernel.NewProc("TerminateJobObject").Call(uintptr(p.job), 1)
	if r == 0 {
		return e
	}
	type accounting struct {
		Times                                 [4]int64
		PageFaults, Total, Active, Terminated uint32
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var info accounting
		r, _, e = kernel.NewProc("QueryInformationJobObject").Call(uintptr(p.job), 1, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0)
		if r == 0 {
			return e
		}
		if info.Active == 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("Windows job termination did not finish")
}
func (p *processController) Close() {
	if p.job != 0 {
		syscall.CloseHandle(p.job)
		p.job = 0
	}
}

// Finish makes the job quiescent before workspace evidence is captured.
func (p *processController) Finish() error { return p.Stop() }
