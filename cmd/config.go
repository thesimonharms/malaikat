package cmd

import (
	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/hw"
	"github.com/simon/malaikat/internal/optimize"
)

func loadConfig() (config.Config, error) {
	return config.Load()
}

func profileFor(cfg config.Config) (optimize.Profile, hw.Info, error) {
	info, err := hw.Detect()
	if err != nil {
		return optimize.Profile{}, info, err
	}
	return optimize.ForHost(info, cfg), info, nil
}
