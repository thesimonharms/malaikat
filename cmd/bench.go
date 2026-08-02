package cmd

import (
	"fmt"

	"github.com/simon/malaikat/internal/engine"
)

func runBench(args []string) error {
	fs := newFlagSet("bench")
	model := modelFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	modelPath, err := resolveModel(*model)
	if err != nil {
		return err
	}
	inst, err := engine.Current()
	if err != nil {
		return err
	}
	if inst.BenchExe == "" {
		return fmt.Errorf("llama-bench not found in runtime; try: malaikat setup --force")
	}
	profile, info, err := profileFor(cfg)
	if err != nil {
		return err
	}

	benchArgs := engine.BuildBenchArgs(modelPath, profile)
	fmt.Println("malaikat bench — Strix Halo Vulkan profile")
	if info.IsStrixHalo {
		fmt.Println("Hardware:", info.String())
	}
	fmt.Println("Runtime: ", inst.Tag)
	fmt.Println("Model:   ", modelPath)
	fmt.Println("Args:    ", benchArgs)
	fmt.Println()

	cmd := engine.Command(inst.BenchExe, benchArgs, profile)
	return cmd.Run()
}
