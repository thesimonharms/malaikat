package cmd

import (
	"fmt"
	"os"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/engine"
)

func runSetup(args []string) error {
	fs := newFlagSet("setup")
	force := fs.Bool("force", false, "re-build / re-download even if current")
	bundle := fs.Bool("bundle", false, "use prebuilt lemonade-sdk ROCm bundle instead of building from source")
	source := fs.Bool("source", false, "build llama.cpp from source against system ROCm (default)")
	ref := fs.String("ref", "", "llama.cpp tag/branch to build, e.g. b1311 (source only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundle && *source {
		return fmt.Errorf("choose one: --bundle or --source")
	}
	if *ref != "" && *bundle {
		return fmt.Errorf("--ref only applies to --source builds")
	}

	// Default: build from source against the system ROCm. All-in-one
	// (embedded runtime) builds default to extracting the bundle instead.
	useSource := *source || (!*bundle && !engine.HasEmbeddedROCm())

	var (
		inst engine.Install
		err  error
	)
	if useSource {
		fmt.Println("Building llama.cpp from source against system ROCm (HIP gfx1151)...")
		inst, err = engine.EnsureSource(*force, *ref)
	} else if engine.HasEmbeddedROCm() {
		fmt.Println("Installing bundled lemonade-sdk llamacpp-rocm Ubuntu gfx1151 build...")
		inst, err = engine.EnsureROCm(*force)
	} else {
		fmt.Println("Installing lemonade-sdk llamacpp-rocm Ubuntu gfx1151 bundle...")
		inst, err = engine.EnsureROCm(*force)
	}
	if err != nil {
		return err
	}

	fmt.Printf("Ready: %s (%s)\n", inst.Tag, inst.Backend)
	fmt.Printf("Server: %s\n", inst.ServerExe)
	if err := engine.SmokeTest(inst); err != nil {
		fmt.Fprintf(os.Stderr, "warning: GPU smoke test failed: %v\n", err)
	}
	if dir, err := config.ModelsDir(); err == nil {
		fmt.Println("Models:", dir)
	}
	fmt.Println("Verify MTP: malaikat serve will pass --spec-type draft-mtp")
	fmt.Println("Next: malaikat serve -m path/to/model.gguf")
	return nil
}
