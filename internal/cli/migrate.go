package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type legacyHostConfig struct {
	Bustable         bool           `yaml:"bustable"`
	Transform        bool           `yaml:"transform"`
	TransformPresets map[string]int `yaml:"transform_presets"`
	Optimize         bool           `yaml:"optimize"`
	Origins          yaml.Node      `yaml:"origins"`
}

type legacyConfig struct {
	Version string                      `yaml:"version"`
	Listen  string                      `yaml:"listen"`
	Logger  yaml.Node                   `yaml:"logger"`
	Cache   yaml.Node                   `yaml:"cache"`
	Hosts   map[string]legacyHostConfig `yaml:"hosts"`
}

type v2TransformsConfig struct {
	Optimize      bool           `yaml:"optimize"`
	Lossless      bool           `yaml:"lossless"`
	Resize        bool           `yaml:"resize"`
	ResizePresets map[string]int `yaml:"resize_presets"`
}

type v2HostConfig struct {
	Bustable   bool               `yaml:"bustable"`
	Transforms v2TransformsConfig `yaml:"transforms"`
	Origins    yaml.Node          `yaml:"origins"`
}

type v2Config struct {
	Version string                  `yaml:"version"`
	Listen  string                  `yaml:"listen"`
	Logger  yaml.Node               `yaml:"logger"`
	Cache   yaml.Node               `yaml:"cache"`
	Hosts   map[string]v2HostConfig `yaml:"hosts"`
}

var migrateCmd = &cobra.Command{
	Use:   "migrate [config-file]",
	Short: "Migrate configuration to v2",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrate(args[0])
	},
}

func runMigrate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var rawCfg legacyConfig
	if err := yaml.Unmarshal(data, &rawCfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	if rawCfg.Version == "2" {
		return nil
	}

	newCfg := v2Config{
		Version: "2",
		Listen:  rawCfg.Listen,
		Logger:  rawCfg.Logger,
		Cache:   rawCfg.Cache,
		Hosts:   make(map[string]v2HostConfig),
	}

	for host, oldHost := range rawCfg.Hosts {
		newHost := v2HostConfig{
			Bustable: oldHost.Bustable,
			Origins:  oldHost.Origins,
			Transforms: v2TransformsConfig{
				Optimize:      oldHost.Optimize,
				Resize:        oldHost.Transform,
				ResizePresets: oldHost.TransformPresets,
			},
		}

		newCfg.Hosts[host] = newHost
	}

	newData, err := yaml.Marshal(&newCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal new config: %w", err)
	}

	if err := os.WriteFile(path, newData, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
