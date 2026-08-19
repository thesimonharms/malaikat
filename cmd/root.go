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
	lastHint := lastPathHint()
	fmt.Fprintf(os.Stderr, `malaikat — personal Strix Halo ROCm MoE+MTP llama-server launcher

Usage:
  malaikat setup [--force] [--bundle | --source] [--ref TAG]
  malaikat serve
  malaikat serve [-config file] [-m model.gguf] [flags] [-- extra llama-server args]
  malaikat bench [-config file] [-m model.gguf] [-url URL] [-ollama MODEL]
  malaikat version

Bare "malaikat serve" reloads the last successful settings from:
  %s

CLI flags override -config; -config (or first run) overrides last.yaml.
`, lastHint)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// loadServeConfig resolves settings in order: last.yaml → -config file → CLI.
// When -config is set, that file replaces last.yaml as the base.
func loadServeConfig(configPath, model string, apply func(*config.Config)) (config.Config, string, error) {
	var (
		cfg    config.Config
		source string
		err    error
	)

	if strings.TrimSpace(configPath) != "" {
		cfg, err = config.MergeFile(configPath)
		if err != nil {
			return cfg, "", err
		}
		source = configPath
	} else if last, ok, lerr := config.LoadLast(); lerr != nil {
		return last, "", lerr
	} else if ok {
		cfg = last
		source, _ = config.LastPath()
	} else {
		cfg = config.Default()
		source = "defaults"
	}

	if apply != nil {
		apply(&cfg)
	}
	if strings.TrimSpace(model) != "" {
		cfg.ModelPath = model
	}
	cfg.ApplyDefaults()
	return cfg, source, nil
}

func loadMergedConfig(configPath, model string, apply func(*config.Config)) (config.Config, error) {
	cfg, _, err := loadServeConfig(configPath, model, apply)
	return cfg, err
}

func requireModel(cfg config.Config) (string, error) {
	m := config.ExpandPath(strings.TrimSpace(cfg.ModelPath))
	if m == "" {
		return "", fmt.Errorf("model required: run with -m / -config once, or set model in %s", mustLastPath())
	}
	if _, err := os.Stat(m); err != nil {
		return "", fmt.Errorf("model not found: %s", m)
	}
	return m, nil
}

func mustLastPath() string {
	return lastPathHint()
}

func lastPathHint() string {
	if p, err := config.LastPath(); err == nil {
		return p
	}
	return "~/.config/malaikat/last.yaml"
}

func splitPassthrough(args []string) (flags, extra []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}
