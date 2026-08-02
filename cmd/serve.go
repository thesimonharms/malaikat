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
)

func runServe(args []string) error {
	fs := newFlagSet("serve")
	model := modelFlag(fs)
	host := fs.String("host", "", "bind host")
	port := fs.Int("port", 0, "bind port")
	ctxSize := fs.Int("c", 0, "context size")
	ngl := fs.Int("ngl", -1, "GPU layers (-1 = config/default)")
	batch := fs.Int("b", 0, "batch size")
	ubatch := fs.Int("ub", 0, "ubatch size")
	save := fs.Bool("save", false, "persist model path / overrides to config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if *host != "" {
		cfg.Host = *host
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *ctxSize != 0 {
		cfg.CtxSize = *ctxSize
	}
	if *ngl >= 0 {
		cfg.NGL = *ngl
	}
	if *batch != 0 {
		cfg.Batch = *batch
	}
	if *ubatch != 0 {
		cfg.UBatch = *ubatch
	}

	modelPath, err := resolveModel(*model)
	if err != nil {
		return err
	}
	if *save {
		cfg.ModelPath = modelPath
		if err := config.Save(cfg); err != nil {
			return err
		}
	}

	inst, err := engine.Current()
	if err != nil {
		return err
	}
	profile, info, err := profileFor(cfg)
	if err != nil {
		return err
	}

	opts := engine.ServerOpts{
		Model:   modelPath,
		Host:    cfg.Host,
		Port:    cfg.Port,
		Profile: profile,
	}
	argsServer := engine.BuildServerArgs(opts)

	fmt.Println("malaikat serve — Strix Halo Vulkan profile")
	if info.IsStrixHalo {
		fmt.Println("Hardware:", info.String())
	}
	fmt.Println("Runtime: ", inst.Tag, "("+inst.Backend+")")
	fmt.Println("Model:   ", modelPath)
	fmt.Println("Listen:  ", opts.BaseURL())
	fmt.Println("Args:    ", inst.ServerExe, argsServer)
	fmt.Println()

	cmd := engine.Command(inst.ServerExe, argsServer, profile)
	if err := cmd.Start(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	go func() {
		if err := engine.WaitReady(ctx, opts.BaseURL()); err == nil {
			fmt.Printf("Ready — OpenAI-compatible API at %s/v1\n", opts.BaseURL())
			fmt.Printf("Health: %s/health\n", opts.BaseURL())
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		_ = cmd.Process.Signal(os.Interrupt)
		time.Sleep(2 * time.Second)
		_ = cmd.Process.Kill()
	}()

	return cmd.Wait()
}
