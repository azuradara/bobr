package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type LoggerConfig struct {
	Level string `yaml:"level"`
}

type CacheConfig struct {
	Dir           string `yaml:"dir"`
	DbDir         string `yaml:"db_dir"`
	MaxSize       string `yaml:"max_size"`
	MaxObjectSize string `yaml:"max_object_size"`
}

type OriginConfig struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type"`
	Prefix     string            `yaml:"prefix"`
	Transforms *TransformsConfig `yaml:"transforms,omitempty"`
	Config     map[string]string `yaml:"config"`
}

type HostConfig struct {
	Bustable   bool             `yaml:"bustable"`
	Transforms TransformsConfig `yaml:"transforms"`
	Origins    []OriginConfig   `yaml:"origins"`
}

type TransformsConfig struct {
	Optimize      bool           `yaml:"optimize"`
	Lossless      bool           `yaml:"lossless"`
	Resize        bool           `yaml:"resize"`
	ResizePresets map[string]int `yaml:"resize_presets"`
}

type Config struct {
	Version string                `yaml:"version"`
	Listen  string                `yaml:"listen"`
	Logger  LoggerConfig          `yaml:"logger"`
	Cache   CacheConfig           `yaml:"cache"`
	Hosts   map[string]HostConfig `yaml:"hosts"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	for host, hostCfg := range c.Hosts {
		hasRoot := false
		hasPrefix := false

		for _, origin := range hostCfg.Origins {
			prefix := origin.Prefix
			if prefix == "" || prefix == "/" {
				hasRoot = true
			} else {
				hasPrefix = true

				if !strings.HasPrefix(prefix, "/") {
					return fmt.Errorf("host %s: origin prefix '%s' must start with /", host, prefix)
				}
				trimmed := strings.TrimPrefix(prefix, "/")
				if strings.Contains(trimmed, "/") || trimmed == "" {
					return fmt.Errorf("host %s: origin prefix '%s' must be exactly one level deep (e.g., /foo)", host, prefix)
				}
			}
		}

		if hasRoot && hasPrefix {
			return errors.New("cannot mix root paths and prefixed paths for the same host: " + host)
		}
	}

	return nil
}
