package cmd

import (
	"fmt"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/engine"
	"github.com/simon/malaikat/internal/hw"
	"github.com/simon/malaikat/internal/optimize"
)

func runSetup(args []string) error {
	fs := newFlagSet("setup")
	force := fs.Bool("force", false, "re-download even if current")
	if err := fs.Parse(args); err != nil {
		return err
	}

	info, err := hw.Detect()
	if err != nil {
		return err
	}
	if info.IsStrixHalo {
		fmt.Println("Strix Halo detected — installing latest llama.cpp Windows Vulkan build.")
	} else {
		fmt.Println("Installing latest llama.cpp Windows Vulkan build (recommended for AMD on Windows).")
	}
	if !info.VulkanAvailable {
		fmt.Println("Warning: Vulkan runtime not clearly detected. Install AMD Adrenalin first.")
	}

	inst, err := engine.EnsureVulkan(*force)
	if err != nil {
		return err
	}

	cfg, _ := config.Load()
	cfg.LlamaDir = inst.Dir
	cfg.LlamaTag = inst.Tag
	if err := config.Save(cfg); err != nil {
		return err
	}

	profile := optimize.ForHost(info, cfg)
	fmt.Println()
	fmt.Printf("Ready: %s @ %s\n", inst.Tag, inst.Dir)
	fmt.Printf("Default serve flags: -ngl %d -b %d -ub %d -fa on -t %d\n",
		profile.NGL, profile.Batch, profile.UBatch, profile.Threads)
	fmt.Println("Run: malaikat serve -m path\\to\\model.gguf")
	return nil
}
