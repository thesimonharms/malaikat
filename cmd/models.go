package cmd

import (
	"fmt"
	"strings"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/hw"
	"github.com/simon/malaikat/internal/optimize"
)

func runModels(args []string) error {
	fs := newFlagSet("models")
	if err := fs.Parse(args); err != nil {
		return err
	}

	info, err := hw.Detect()
	if err != nil {
		return err
	}
	modelsDir, _ := config.ModelsDir()

	fmt.Println("Suggested GGUFs for this machine")
	fmt.Println(strings.Repeat("─", 56))
	fmt.Printf("RAM: %.0f GB | recommended VGM: %d GB\n", info.TotalRAMGB, info.RecommendedVGMGB())
	fmt.Printf("Local models dir: %s\n\n", modelsDir)
	for _, s := range optimize.SuggestModels(info) {
		fmt.Println("•", s)
	}
	fmt.Println()
	fmt.Println("Download any GGUF (Hugging Face / huggingface-cli), then:")
	fmt.Println("  malaikat serve -m path\\to\\model.gguf --save")
	fmt.Println()
	fmt.Println("Speed rule on Strix Halo: MoE ≫ dense at the same total parameter count")
	fmt.Println("because generation is ~215 GB/s memory-bandwidth limited.")
	return nil
}
