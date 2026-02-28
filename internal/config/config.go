package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	DatabasePath string   `json:"database_path"`
	ServerAddr   string   `json:"server_addr"`
	ScanPaths    []string `json:"scan_paths"`
}

func GetDefaultDataDir() string {
	userConfig, _ := os.UserConfigDir()
	if userConfig == "" {
		return "."
	}
	dataDir := filepath.Join(userConfig, "VDFusion")
	_ = os.MkdirAll(dataDir, 0755)
	return dataDir
}

func Load() *Config {
	dataDir := GetDefaultDataDir()
	// Default values
	cfg := &Config{
		DatabasePath: filepath.Join(dataDir, "vdf.db"),
		ServerAddr:   ":8080",
		ScanPaths:    []string{},
	}

	// Try loading from JSON if exists
	data, err := os.ReadFile("config.json")
	if err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	// Override with environment variables if present
	if dbPath := os.Getenv("VDF_DB_PATH"); dbPath != "" {
		cfg.DatabasePath = dbPath
	}
	if addr := os.Getenv("VDF_SERVER_ADDR"); addr != "" {
		cfg.ServerAddr = addr
	}

	return cfg
}
