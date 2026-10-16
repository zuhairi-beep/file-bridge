package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// Config represents the application configuration
type Config struct {
	SaveDir        string `json:"saveDir"`
	Lang           string `json:"lang"`
	CompressImages bool   `json:"compressImages"`
	ImageQuality   int    `json:"imageQuality"`
	KeepOriginal   bool   `json:"keepOriginal"`
	// Port is the fixed HTTP port (default 8765) so an iOS Shortcut keeps a stable URL.
	Port int `json:"port,omitempty"`
	// ShortcutToken authenticates /api/shortcut/upload (header X-FileBridge-Token).
	ShortcutToken string `json:"shortcutToken,omitempty"`
}

// defaultPort is used unless config.json sets another port.
const defaultPort = 8765

// ensureShortcutToken creates and persists a random token on first run.
func ensureShortcutToken(cfg *Config) {
	if cfg.ShortcutToken != "" {
		return
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return
	}
	cfg.ShortcutToken = hex.EncodeToString(b)
	if err := SaveConfig(cfg); err != nil {
		log.Printf("Failed to save shortcut token: %v", err)
	}
}

// configFileName is the config file name
const configFileName = "config.json"

// getConfigDir returns the config directory path
func getConfigDir() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appData, "FileBridge")
}

// getConfigPath returns the full config file path
func getConfigPath() string {
	return filepath.Join(getConfigDir(), configFileName)
}

// LoadConfig loads config from disk
func LoadConfig() *Config {
	cfg := &Config{}

	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		log.Printf("No config file found, using defaults: %v", err)
		// Default save directory: user's Downloads folder
		home, _ := os.UserHomeDir()
		cfg.SaveDir = filepath.Join(home, "Downloads", "FileBridge")
		cfg.Lang = "ja"
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		log.Printf("Failed to parse config: %v", err)
		home, _ := os.UserHomeDir()
		cfg.SaveDir = filepath.Join(home, "Downloads", "FileBridge")
	}

	if cfg.Lang == "" {
		cfg.Lang = "ja"
	}

	// Default image quality
	if cfg.ImageQuality == 0 {
		cfg.ImageQuality = 80
	}

	return cfg
}

// SaveConfig saves config to disk
func SaveConfig(cfg *Config) error {
	dir := getConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(getConfigPath(), data, 0644)
}
