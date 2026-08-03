package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/engine"
	"github.com/simon/malaikat/internal/optimize"
)

func runServe(args []string) error {
	flags, passthrough := splitPassthrough(args)
	fs := newFlagSet("serve")
	configPath := fs.String("config", "", "JSON/YAML config file")
	model := fs.String("m", "", "GGUF model path")
	host := fs.String("host", "", "bind host")
	port := fs.Int("port", 0, "bind port")
	ctxSize := fs.Int("c", 0, "context size")
	ngl := fs.Int("ngl", -1, "GPU layers")
	batch := fs.Int("b", 0, "batch size")
	ubatch := fs.Int("ub", 0, "ubatch size")
	specType := fs.String("spec-type", "", "speculative type (default draft-mtp)")
	draftMax := fs.Int("spec-draft-n-max", -1, "MTP draft n-max")
	fa := fs.String("fa", "", "flash-attn on|off")
	noMTP := fs.Bool("no-mtp", false, "disable MTP speculative decode")
	if err := fs.Parse(flags); err != nil {
		return err
	}

	cfg, err := loadMergedConfig(*configPath, *model, func(c *config.Config) {
		if *host != "" {
			c.Host = *host
		}
		if *port != 0 {
			c.Port = *port
		}
		if *ctxSize != 0 {
			c.CtxSize = *ctxSize
		}
		if *ngl >= 0 {
			c.NGL = *ngl
		}
		if *batch != 0 {
			c.Batch = *batch
		}
		if *ubatch != 0 {
			c.UBatch = *ubatch
		}
		if *specType != "" {
			c.SpecType = *specType
		}
		if *draftMax >= 0 {
			c.SpecDraftNMax = *draftMax
		}
		if *fa != "" {
			c.FlashAttn = *fa
		}
		if *noMTP {
			c.SpecType = "none"
		}
	})
	if err != nil {
		return err
	}

	modelPath, err := requireModel(cfg)
	if err != nil {
		return err
	}

	inst, err := engine.Current()
	if err != nil {
		return err
	}
	profile := optimize.FromConfig(cfg)
	opts := engine.ServerOpts{
		Model:   modelPath,
		Host:    cfg.Host,
		Port:    cfg.Port,
		Profile: profile,
		Extra:   passthrough,
	}
	argv := engine.BuildServerArgs(opts)

	fmt.Println("malaikat serve")
	fmt.Println("runtime:", inst.Tag, "("+inst.Backend+")")
	fmt.Println("model:  ", modelPath)
	fmt.Println("listen: ", opts.BaseURL())
	fmt.Println("argv:   ", engine.FormatArgs(inst.ServerExe, argv))
	fmt.Println()

	cmd := engine.Command(inst, argv, profile)
	if err := cmd.Start(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	go func() {
		if err := engine.WaitReady(ctx, opts.BaseURL()); err == nil {
			fmt.Printf("ready %s/v1\n", opts.BaseURL())
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nshutting down...")
		_ = cmd.Process.Signal(os.Interrupt)
		time.Sleep(2 * time.Second)
		_ = cmd.Process.Kill()
	}()

	return cmd.Wait()
}
