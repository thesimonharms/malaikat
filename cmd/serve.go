package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
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
	alias := fs.String("alias", "", "API model id alias")
	host := fs.String("host", "", "bind host")
	port := fs.Int("port", 0, "bind port")
	ctxSize := fs.String("c", "", "context size: 256k, 512k, 1m (0 = model max)")
	ngl := fs.Int("ngl", -1, "GPU layers")
	batch := fs.Int("b", 0, "batch size")
	ubatch := fs.Int("ub", 0, "ubatch size")
	specType := fs.String("spec-type", "", "speculative type (default draft-mtp)")
	draftMax := fs.Int("spec-draft-n-max", -1, "MTP draft n-max")
	fa := fs.String("fa", "", "flash-attn on|off")
	noMTP := fs.Bool("no-mtp", false, "disable MTP speculative decode")
	noSave := fs.Bool("no-save", false, "do not remember these settings as last-used")
	if err := fs.Parse(flags); err != nil {
		return err
	}

	var ctxOverride *config.CtxSize
	if strings.TrimSpace(*ctxSize) != "" {
		n, err := config.ParseCtxSize(*ctxSize)
		if err != nil {
			return err
		}
		v := config.CtxSize(n)
		ctxOverride = &v
	}

	cfg, source, err := loadServeConfig(*configPath, *model, func(c *config.Config) {
		if *host != "" {
			c.Host = *host
		}
		if *port != 0 {
			c.Port = *port
		}
		if *alias != "" {
			c.Alias = *alias
		}
		if ctxOverride != nil {
			c.CtxSize = *ctxOverride
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

	// Passthrough after -- becomes remembered extra_args when we save.
	if len(passthrough) > 0 {
		cfg.ExtraArgs = append([]string{}, passthrough...)
	}

	modelPath, err := requireModel(cfg)
	if err != nil {
		return err
	}
	cfg.ModelPath = modelPath

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
		// Passthrough is already on the profile as ExtraArgs when present.
		Extra: nil,
	}
	argv := engine.BuildServerArgs(opts)

	fmt.Println("malaikat serve")
	fmt.Println("platform:", runtime.GOOS)
	fmt.Println("settings:", source)
	fmt.Println("runtime:", inst.Tag, "("+inst.Backend+")")
	fmt.Println("model:  ", modelPath)
	fmt.Println("context:", config.FormatCtxSize(profile.CtxSize))
	if profile.YarnScale > 1 {
		fmt.Printf("yarn:    %.4gx from %d (--rope-scaling yarn)\n", profile.YarnScale, profile.YarnOrigCtx)
	}
	fmt.Println("listen: ", opts.BaseURL())
	fmt.Println("argv:   ", engine.FormatArgs(inst.ServerExe, argv))
	fmt.Println()

	if !*noSave {
		if err := config.SaveLast(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save last settings: %v\n", err)
		} else if p, err := config.LastPath(); err == nil {
			fmt.Println("remembered:", p)
		}
	}

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
