package cmd

import (
	"fmt"
	"strings"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/engine"
	"github.com/simon/malaikat/internal/hw"
	"github.com/simon/malaikat/internal/optimize"
)

func runDoctor(args []string) error {
	fs := newFlagSet("doctor")
	if err := fs.Parse(args); err != nil {
		return err
	}

	info, err := hw.Detect()
	if err != nil {
		return err
	}
	cfg, _ := config.Load()
	profile := optimize.ForHost(info, cfg)

	fmt.Println("malaikat doctor")
	fmt.Println(strings.Repeat("─", 56))
	fmt.Printf("OS:              %s\n", info.OS)
	fmt.Printf("CPU:             %s\n", info.CPUName)
	fmt.Printf("GPU:             %s\n", info.GPUName)
	fmt.Printf("System RAM:      %.1f GB\n", info.TotalRAMGB)
	if info.GPUMemoryMB > 0 {
		fmt.Printf("GPU memory:      %.1f GB (VGM / driver qwMemorySize)\n", float64(info.GPUMemoryMB)/1024)
	} else {
		fmt.Printf("GPU memory:      unknown / 0\n")
	}
	fmt.Printf("Strix Halo:      %v\n", info.IsStrixHalo)
	fmt.Printf("AMD GPU:         %v\n", info.IsAMD)
	fmt.Printf("Vulkan runtime:  %v\n", info.VulkanAvailable)
	fmt.Printf("Recommended VGM: %d GB (leave OS headroom)\n", info.RecommendedVGMGB())
	fmt.Println()
	fmt.Println("Tuned profile")
	fmt.Println(strings.Repeat("─", 56))
	fmt.Printf("backend:         %s\n", profile.Backend)
	fmt.Printf("n-gpu-layers:    %d\n", profile.NGL)
	fmt.Printf("batch / ubatch:  %d / %d\n", profile.Batch, profile.UBatch)
	fmt.Printf("ctx / threads:   %d / %d\n", profile.CtxSize, profile.Threads)
	fmt.Printf("flash-attn:      %v\n", profile.FlashAttn)
	fmt.Printf("high-priority:   %v\n", profile.HighPriority)

	if inst, err := engine.Current(); err == nil {
		fmt.Println()
		fmt.Println("Runtime")
		fmt.Println(strings.Repeat("─", 56))
		fmt.Printf("llama.cpp:       %s (%s)\n", inst.Tag, inst.Backend)
		fmt.Printf("server:          %s\n", inst.ServerExe)
	} else {
		fmt.Println()
		fmt.Println("Runtime: not installed — run `malaikat setup`")
	}

	if len(info.Notes) > 0 {
		fmt.Println()
		fmt.Println("Notes")
		fmt.Println(strings.Repeat("─", 56))
		for _, n := range info.Notes {
			fmt.Println("•", n)
		}
		for _, r := range profile.Rationale {
			fmt.Println("•", r)
		}
	}
	if len(info.Warnings) > 0 {
		fmt.Println()
		fmt.Println("Warnings")
		fmt.Println(strings.Repeat("─", 56))
		for _, w := range info.Warnings {
			fmt.Println("!", w)
		}
	}

	fmt.Println()
	fmt.Println("Next:")
	if info.GPUMemoryMB > 0 && info.GPUMemoryMB >= hw.MinUsefulGPUMemoryMB() {
		fmt.Println("  1. malaikat setup")
		fmt.Println("  2. malaikat serve -m path\\to\\model.gguf")
	} else {
		fmt.Println("  1. Raise Variable Graphics Memory in AMD Adrenalin, reboot")
		fmt.Println("  2. malaikat setup")
		fmt.Println("  3. malaikat serve -m path\\to\\model.gguf")
	}
	return nil
}
