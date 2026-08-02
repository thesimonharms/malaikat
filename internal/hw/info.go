package hw

import "fmt"

// Info describes the host machine for Strix Halo tuning.
type Info struct {
	OS              string
	CPUName         string
	GPUName         string
	TotalRAMGB      float64
	GPUMemoryMB     int64
	IsStrixHalo     bool
	IsAMD           bool
	VulkanAvailable bool
	Notes           []string
	Warnings        []string
}

func (i Info) String() string {
	return fmt.Sprintf("%s / %s / %.0f GB RAM", i.CPUName, i.GPUName, i.TotalRAMGB)
}

// RecommendedVGMGB returns a suggested Variable Graphics Memory size for Windows.
// Leaves ~32 GB for the OS when possible (AMD's common guidance on 128 GB systems).
func (i Info) RecommendedVGMGB() int {
	total := int(i.TotalRAMGB)
	if total <= 32 {
		return max(8, total/2)
	}
	if total <= 64 {
		return total - 24
	}
	if total <= 96 {
		return total - 28
	}
	return total - 32
}

// MinUsefulGPUMemoryMB is the floor before we warn about VGM.
func MinUsefulGPUMemoryMB() int64 {
	return 8 * 1024
}
