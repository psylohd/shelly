package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Shelly  ShellyConfig           `json:"shelly"`
	Shells  map[string]Shell       `json:"shells"`
	Toolbox map[string]ToolboxItem `json:"toolbox"`
}

type ShellyConfig struct {
	DefaultHTTPSvr int `json:"default_http_svr"`
}

type Shell struct {
	Listener  string   `json:"listener"`
	Serve     []string `json:"serve"`
	Templates []string `json:"templates"`
}

type ToolboxItem map[string]ToolboxDetails

type ToolboxDetails struct {
	Filename string `json:"filename"`
	Download string `json:"download"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
