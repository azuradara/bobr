package config

import (
	"os"

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
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type"`
	Config map[string]string `yaml:"config"`
}

type HostConfig struct {
	Bustable bool           `yaml:"bustable"`
	Origins  []OriginConfig `yaml:"origins"`
}

type Config struct {
	Listen string                `yaml:"listen"`
	Logger LoggerConfig          `yaml:"logger"`
	Cache  CacheConfig           `yaml:"cache"`
	Hosts  map[string]HostConfig `yaml:"hosts"`
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

	return &cfg, nil
}
