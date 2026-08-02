package cmd

import (
	"fmt"

	"github.com/simon/malaikat/internal/engine"
)

func runChat(args []string) error {
	fs := newFlagSet("chat")
	model := modelFlag(fs)
	ctxSize := fs.Int("c", 0, "context size")
	ngl := fs.Int("ngl", -1, "GPU layers")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if *ctxSize != 0 {
		cfg.CtxSize = *ctxSize
	}
	if *ngl >= 0 {
		cfg.NGL = *ngl
	}

	modelPath, err := resolveModel(*model)
	if err != nil {
		return err
	}
	inst, err := engine.Current()
	if err != nil {
		return err
	}
	if inst.CLIExe == "" {
		return fmt.Errorf("llama-cli not found in runtime; try: malaikat setup --force")
	}
	profile, info, err := profileFor(cfg)
	if err != nil {
		return err
	}

	cliArgs := engine.BuildCLIArgs(modelPath, profile, nil)
	fmt.Println("malaikat chat — Strix Halo Vulkan profile")
	if info.IsStrixHalo {
		fmt.Println("Hardware:", info.String())
	}
	fmt.Println("Runtime: ", inst.Tag)
	fmt.Println("Model:   ", modelPath)
	fmt.Println()

	cmd := engine.Command(inst.CLIExe, cliArgs, profile)
	return cmd.Run()
}
