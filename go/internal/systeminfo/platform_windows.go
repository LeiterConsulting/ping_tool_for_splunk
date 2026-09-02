//go:build windows

package systeminfo

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	psapi                     = windows.NewLazySystemDLL("psapi.dll")
	procGlobalMemoryStatusEx  = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetProcessHandleCount = kernel32.NewProc("GetProcessHandleCount")
	procGetSystemTimes        = kernel32.NewProc("GetSystemTimes")
	procGetProcessMemoryInfo  = psapi.NewProc("GetProcessMemoryInfo")
)

type memoryStatusEx struct {
	Length                   uint32
	MemoryLoad               uint32
	TotalPhysical            uint64
	AvailablePhysical        uint64
	TotalPageFile            uint64
	AvailablePageFile        uint64
	TotalVirtual             uint64
	AvailableVirtual         uint64
	AvailableExtendedVirtual uint64
}

type processMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

func collectPlatform(previous platformCounters, elapsed time.Duration) (ProcessSnapshot, HostSnapshot, platformCounters, []string) {
	process := ProcessSnapshot{PID: os.Getpid()}
	host := HostSnapshot{}
	counters := platformCounters{}
	unavailable := make([]string, 0)
	handle := windows.CurrentProcess()

	var memory processMemoryCountersEx
	memory.CB = uint32(unsafe.Sizeof(memory))
	if result, _, callErr := procGetProcessMemoryInfo.Call(
		uintptr(handle), uintptr(unsafe.Pointer(&memory)), uintptr(memory.CB),
	); result != 0 {
		process.WorkingSetBytes = uint64Pointer(uint64(memory.WorkingSetSize))
		process.PeakWorkingSetBytes = uint64Pointer(uint64(memory.PeakWorkingSetSize))
		process.PrivateBytes = uint64Pointer(uint64(memory.PrivateUsage))
		process.CommittedBytes = uint64Pointer(uint64(memory.PagefileUsage))
	} else {
		unavailable = append(unavailable, "process_memory: "+windowsCallError(callErr))
	}

	var handleCount uint32
	if result, _, callErr := procGetProcessHandleCount.Call(uintptr(handle), uintptr(unsafe.Pointer(&handleCount))); result != 0 {
		process.HandleCount = uint64Pointer(uint64(handleCount))
	} else {
		unavailable = append(unavailable, "process_handles: "+windowsCallError(callErr))
	}

	if threadCount, err := currentProcessThreadCount(); err == nil {
		process.ThreadCount = uint64Pointer(threadCount)
	} else {
		unavailable = append(unavailable, "process_threads: "+err.Error())
	}

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err == nil {
		counters.processCPU100ns = filetimeTicks(kernel) + filetimeTicks(user)
		counters.validProcessCPU = true
		if previous.validProcessCPU && elapsed > 0 && counters.processCPU100ns >= previous.processCPU100ns {
			deltaCPU := float64(counters.processCPU100ns - previous.processCPU100ns)
			wall100ns := float64(elapsed.Nanoseconds()) / 100
			coreEquivalent := deltaCPU / wall100ns * 100
			hostCapacity := coreEquivalent / float64(max(1, runtime.NumCPU()))
			process.CPUCoreEquivalentPercent = float64Pointer(max(0, coreEquivalent))
			process.CPUHostCapacityPercent = float64Pointer(clampPercent(hostCapacity))
		} else {
			unavailable = append(unavailable, "process_cpu: awaiting a second timed sample")
		}
	} else {
		unavailable = append(unavailable, "process_cpu: "+err.Error())
	}

	var idleTime, kernelTime, userTime windows.Filetime
	if result, _, callErr := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)), uintptr(unsafe.Pointer(&kernelTime)), uintptr(unsafe.Pointer(&userTime)),
	); result != 0 {
		counters.systemIdle100ns = filetimeTicks(idleTime)
		counters.systemKernel100ns = filetimeTicks(kernelTime)
		counters.systemUser100ns = filetimeTicks(userTime)
		counters.validSystemCPU = true
		if previous.validSystemCPU &&
			counters.systemIdle100ns >= previous.systemIdle100ns &&
			counters.systemKernel100ns >= previous.systemKernel100ns &&
			counters.systemUser100ns >= previous.systemUser100ns {
			idleDelta := counters.systemIdle100ns - previous.systemIdle100ns
			kernelDelta := counters.systemKernel100ns - previous.systemKernel100ns
			userDelta := counters.systemUser100ns - previous.systemUser100ns
			total := kernelDelta + userDelta
			if total > 0 && total >= idleDelta {
				host.CPUPercent = float64Pointer(clampPercent(float64(total-idleDelta) / float64(total) * 100))
			} else {
				unavailable = append(unavailable, "host_cpu: timed sample contained no measurable scheduler interval")
			}
		} else {
			unavailable = append(unavailable, "host_cpu: awaiting a second timed sample")
		}
	} else {
		unavailable = append(unavailable, "host_cpu: "+windowsCallError(callErr))
	}

	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	if result, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status))); result != 0 {
		host.TotalMemoryBytes = uint64Pointer(status.TotalPhysical)
		host.AvailableMemoryBytes = uint64Pointer(status.AvailablePhysical)
		if status.TotalPhysical > 0 && status.AvailablePhysical <= status.TotalPhysical {
			host.MemoryUsedPercent = float64Pointer(clampPercent(float64(status.TotalPhysical-status.AvailablePhysical) / float64(status.TotalPhysical) * 100))
		}
	} else {
		unavailable = append(unavailable, "host_memory: "+windowsCallError(callErr))
	}

	return process, host, counters, unavailable
}

func filetimeTicks(value windows.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}

func currentProcessThreadCount() (uint64, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return 0, err
	}
	processID := uint32(os.Getpid())
	var count uint64
	for {
		if entry.OwnerProcessID == processID {
			count++
		}
		err = windows.Thread32Next(snapshot, &entry)
		if err == nil {
			continue
		}
		if err == syscall.ERROR_NO_MORE_FILES {
			return count, nil
		}
		return 0, err
	}
}

func windowsCallError(err error) string {
	if err == nil || err == syscall.Errno(0) {
		return "Windows API returned failure without an error code"
	}
	return fmt.Sprint(err)
}
