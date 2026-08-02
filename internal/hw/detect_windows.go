//go:build windows

package hw

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var (
	strixCPU = regexp.MustCompile(`(?i)(ryzen\s*ai\s*max|strix\s*halo|ryzen\s*ai\s*9\s*hx\s*370)`)
	strixGPU = regexp.MustCompile(`(?i)(radeon.*8060s|radeon.*8050s|radeon.*8040s|gfx1151|strix\s*halo)`)
	amdGPU   = regexp.MustCompile(`(?i)(amd|radeon)`)
)

func Detect() (Info, error) {
	info := Info{
		OS: runtime.GOOS + "/" + runtime.GOARCH,
	}

	info.TotalRAMGB = totalRAMGB()
	info.CPUName = wmiString("Win32_Processor", "Name")
	info.GPUName = bestGPUName()
	info.GPUMemoryMB = gpuMemoryMB(info.GPUName)

	info.IsAMD = amdGPU.MatchString(info.GPUName) || amdGPU.MatchString(info.CPUName)
	info.IsStrixHalo = strixCPU.MatchString(info.CPUName) || strixGPU.MatchString(info.GPUName)
	info.VulkanAvailable = vulkanPresent()

	if info.IsStrixHalo {
		info.Notes = append(info.Notes,
			"Detected AMD Strix Halo-class APU (unified memory).",
			"Fastest Windows path: llama.cpp Vulkan with full GPU offload (-ngl 999).",
			"Prefer MoE models (e.g. Qwen3 30B-A3B) for ~100 t/s; dense 70B is bandwidth-bound (~5 t/s).",
		)
		rec := info.RecommendedVGMGB()
		curGB := int(info.GPUMemoryMB / 1024)
		if info.GPUMemoryMB == 0 {
			info.Warnings = append(info.Warnings,
				"Could not read GPU memory size. Raise Variable Graphics Memory in Adrenalin so Vulkan sees a DEVICE_LOCAL heap, then reboot.",
			)
		} else if info.GPUMemoryMB < MinUsefulGPUMemoryMB() {
			info.Warnings = append(info.Warnings,
				"GPU reports only "+formatMB(info.GPUMemoryMB)+" addressable memory. Raise Variable Graphics Memory in Adrenalin to ~"+strconv.Itoa(rec)+" GB, then reboot.",
			)
		} else if curGB+8 < rec {
			info.Notes = append(info.Notes,
				"Consider raising Adrenalin Variable Graphics Memory toward ~"+strconv.Itoa(rec)+" GB (currently ~"+strconv.Itoa(curGB)+" GB), then reboot.",
			)
		} else {
			info.Notes = append(info.Notes,
				"GPU memory looks usable (~"+formatMB(info.GPUMemoryMB)+"). Keep enough RAM free for Windows while models are loaded.",
			)
		}
	} else if info.IsAMD {
		info.Notes = append(info.Notes, "AMD GPU detected; Vulkan backend will still be used.")
	} else {
		info.Warnings = append(info.Warnings, "Not identified as Strix Halo; defaults still target Vulkan on Windows.")
	}

	if !info.VulkanAvailable {
		info.Warnings = append(info.Warnings,
			"vulkaninfo not found. Install AMD Adrenalin (includes Vulkan) or the Vulkan SDK runtime.",
		)
	}

	return info, nil
}

func totalRAMGB() float64 {
	ps := `[math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1GB, 1)`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return v
}

func wmiString(class, property string) string {
	ps := `Get-CimInstance ` + class + ` | Select-Object -ExpandProperty ` + property
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return "unknown"
	}
	return strings.TrimSpace(lines[0])
}

func bestGPUName() string {
	ps := `(Get-CimInstance Win32_VideoController | Where-Object { $_.Name -match 'AMD|Radeon' } | Select-Object -First 1 -ExpandProperty Name)`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name
		}
	}
	return wmiString("Win32_VideoController", "Name")
}

func gpuMemoryMB(gpuName string) int64 {
	// HardwareInformation.qwMemorySize reflects VGM on Strix Halo; WMI AdapterRAM is a 32-bit field (~4GB cap).
	ps := `
$displayClass = 'HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}'
$want = '` + escapePS(gpuName) + `'
$bytes = 0
Get-ChildItem $displayClass -ErrorAction SilentlyContinue | ForEach-Object {
  $p = Get-ItemProperty $_.PSPath -ErrorAction SilentlyContinue
  if (-not $p.DriverDesc) { return }
  if ($p.DriverDesc -eq $want -or $p.DriverDesc -match 'AMD|Radeon') {
    $q = $p.'HardwareInformation.qwMemorySize'
    if ($q -and [int64]$q -gt $bytes) { $bytes = [int64]$q }
  }
}
if ($bytes -le 0) {
  $g = Get-CimInstance Win32_VideoController | Where-Object { $_.Name -match 'AMD|Radeon' } | Select-Object -First 1
  if ($g -and $g.AdapterRAM -and $g.AdapterRAM -lt 4200000000) { $bytes = [int64]$g.AdapterRAM }
}
[int64]($bytes / 1MB)
`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func vulkanPresent() bool {
	if _, err := exec.LookPath("vulkaninfo"); err == nil {
		return true
	}
	ps := `Test-Path "$env:WINDIR\System32\vulkan-1.dll"`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(strings.ToLower(string(out))) == "true"
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func formatMB(mb int64) string {
	if mb >= 1024 {
		return strconv.FormatFloat(float64(mb)/1024, 'f', 1, 64) + " GB"
	}
	return strconv.FormatInt(mb, 10) + " MB"
}
