package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Backend      string
	BufferLength float64
	Devices      map[string]string
	Bind         string
	Port         int
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
