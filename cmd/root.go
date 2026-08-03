package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/simon/malaikat/internal/config"
)

const version = "0.2.0"

func Execute() error {
	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("missing command")
	}

	switch os.Args[1] {
	case "setup":
		return runSetup(os.Args[2:])
	case "serve":
		return runServe(os.Args[2:])
	case "bench":
		return runBench(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("malaikat", version)
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `malaikat — personal Strix Halo ROCm MoE+MTP llama-server launcher

Usage:
  malaikat setup [--force]
  malaikat serve [-config file] [-m model.gguf] [flags] [-- extra llama-server args]
  malaikat bench [-config file] [-m model.gguf] [-url URL] [-ollama MODEL]
  malaikat version

Defaults: ROCm gfx1151 binary, -ngl 999, MTP draft-mtp n-max=2, MoE-friendly batches.
Config: JSON or YAML. CLI flags override the file.
`)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func loadMergedConfig(configPath, model string, apply func(*config.Config)) (config.Config, error) {
	cfg, err := config.MergeFile(configPath)
	if err != nil {
		return cfg, err
	}
	if apply != nil {
		apply(&cfg)
	}
	if strings.TrimSpace(model) != "" {
		cfg.ModelPath = model
	}
	cfg.ApplyDefaults()
	return cfg, nil
}

func requireModel(cfg config.Config) (string, error) {
	m := strings.TrimSpace(cfg.ModelPath)
	if m == "" {
		return "", fmt.Errorf("model required: -m path.gguf or model: in config file")
	}
	if _, err := os.Stat(m); err != nil {
		return "", fmt.Errorf("model not found: %s", m)
	}
	return m, nil
}

func splitPassthrough(args []string) (flags, extra []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}
