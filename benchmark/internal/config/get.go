package config

import (
	"os"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	Url     string `yaml:"url"`
	Servers map[string]struct {
		ImageName string `yaml:"image_name"`
	} `yaml:"servers"`
}

func GetConfig() *Config {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		panic(err)
	}

	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		panic(err)
	}
	return &cfg
}
