//go:build !windows

package hw

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func Detect() (Info, error) {
	info := Info{
		OS:              runtime.GOOS + "/" + runtime.GOARCH,
		CPUName:         firstLine(readFile("/proc/cpuinfo"), "model name"),
		GPUName:         "unknown",
		TotalRAMGB:      meminfoGB(),
		VulkanAvailable: commandExists("vulkaninfo"),
	}
	if out, err := exec.Command("lspci").CombinedOutput(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(strings.ToLower(line), "vga") || strings.Contains(strings.ToLower(line), "display") {
				info.GPUName = strings.TrimSpace(line)
				break
			}
		}
	}
	low := strings.ToLower(info.CPUName + " " + info.GPUName)
	info.IsAMD = strings.Contains(low, "amd") || strings.Contains(low, "radeon")
	info.IsStrixHalo = strings.Contains(low, "strix") || strings.Contains(low, "ryzen ai max") || strings.Contains(low, "8060s")
	if info.IsStrixHalo {
		info.Notes = append(info.Notes,
			"Strix Halo on non-Windows: prefer llama.cpp Vulkan/RADV; set amdgpu.gttsize appropriately.",
		)
	}
	info.Warnings = append(info.Warnings, "malaikat is optimized for Windows + AMD Strix Halo; other platforms are best-effort.")
	return info, nil
}

func meminfoGB() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0
			}
			kb, _ := strconv.ParseFloat(fields[1], 64)
			return kb / (1024 * 1024)
		}
	}
	return 0
}

func readFile(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	return firstLine(string(data), key)
}

func firstLine(data, key string) string {
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(key)) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
