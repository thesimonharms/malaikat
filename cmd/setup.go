package cmd

import (
	"fmt"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/engine"
)

func runSetup(args []string) error {
	fs := newFlagSet("setup")
	force := fs.Bool("force", false, "re-download even if current")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if engine.HasEmbeddedROCm() {
		fmt.Println("Installing bundled lemonade-sdk llamacpp-rocm Ubuntu gfx1151 build...")
	} else {
		fmt.Println("Installing lemonade-sdk llamacpp-rocm Ubuntu gfx1151 build...")
	}
	inst, err := engine.EnsureROCm(*force)
	if err != nil {
		return err
	}

	fmt.Printf("Ready: %s (%s)\n", inst.Tag, inst.Backend)
	fmt.Printf("Server: %s\n", inst.ServerExe)
	if dir, err := config.ModelsDir(); err == nil {
		fmt.Println("Models:", dir)
	}
	fmt.Println("Verify MTP: malaikat serve will pass --spec-type draft-mtp")
	fmt.Println("Next: malaikat serve -m path/to/model.gguf")
	return nil
}
