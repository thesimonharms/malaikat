package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const version = "0.1.0"

func Execute() error {
	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("missing command")
	}

	switch os.Args[1] {
	case "doctor":
		return runDoctor(os.Args[2:])
	case "setup":
		return runSetup(os.Args[2:])
	case "serve":
		return runServe(os.Args[2:])
	case "chat":
		return runChat(os.Args[2:])
	case "bench":
		return runBench(os.Args[2:])
	case "models":
		return runModels(os.Args[2:])
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
	fmt.Fprintf(os.Stderr, `malaikat — local LLM runner optimized for AMD Strix Halo (Windows + Vulkan)

Usage:
  malaikat <command> [options]

Commands:
  doctor    Detect Strix Halo / VGM / Vulkan readiness
  setup     Download latest llama.cpp Windows Vulkan build
  serve     Start OpenAI-compatible llama-server with tuned flags
  chat      Interactive CLI chat via llama-cli
  bench     Run llama-bench with Strix Halo defaults
  models    Suggest GGUF targets for this machine
  version   Print version

Examples:
  malaikat doctor
  malaikat setup
  malaikat serve -m C:\models\qwen3-30b-a3b-q4_k_m.gguf
  malaikat chat  -m C:\models\qwen3-30b-a3b-q4_k_m.gguf
  malaikat bench -m C:\models\qwen3-30b-a3b-q4_k_m.gguf

Windows tip:
  AMD Adrenalin → Performance → Tuning → Variable Graphics Memory = Custom
  (leave ~32 GB for Windows on 128 GB systems), then reboot.
`)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func modelFlag(fs *flag.FlagSet) *string {
	return fs.String("m", "", "path to GGUF model")
}

func resolveModel(flagVal string) (string, error) {
	m := strings.TrimSpace(flagVal)
	if m == "" {
		cfg, err := loadConfig()
		if err == nil && cfg.ModelPath != "" {
			m = cfg.ModelPath
		}
	}
	if m == "" {
		return "", fmt.Errorf("model required: pass -m path/to/model.gguf (or set model_path in config)")
	}
	if _, err := os.Stat(m); err != nil {
		return "", fmt.Errorf("model not found: %s", m)
	}
	return m, nil
}
