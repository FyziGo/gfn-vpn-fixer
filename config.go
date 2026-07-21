package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the persistent settings for GFN VPN FIXER.
type Config struct {
	// Blacklist contains the friendly names of network adapters to disable before GFN starts.
	Blacklist []string `json:"blacklist"`
	// GFNPath is the absolute path to the GeForceNOW executable.
	GFNPath string `json:"gfn_path"`
	// LaunchGFN controls whether GeForce NOW is started automatically.
	LaunchGFN bool `json:"launch_gfn"`
	// VPNServices lists Windows service names to stop before GFN starts (e.g. "Tailscale").
	VPNServices []string `json:"vpn_services"`
	// BandwidthLimitMbps limits network bandwidth visible to GFN via Windows QoS policy.
	// 0 means no limit is applied.
	BandwidthLimitMbps int `json:"bandwidth_limit_mbps"`
}

// configPath returns the absolute path to config.json located next to the executable.
func configPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "config.json"), nil
}

// loadConfig reads and unmarshals config.json. Returns an empty Config and a non-nil
// error if the file does not exist or is malformed.
func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	// Default LaunchGFN to true so existing configs without the field keep the old behaviour.
	cfg := Config{LaunchGFN: true}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("malformed config.json: %w", err)
	}
	return cfg, nil
}

// saveConfig marshals cfg and writes it to path with 4-space indentation.
func saveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}
	return nil
}

// defaultGFNPath resolves the canonical GeForce NOW executable path for the current user.
// Falls back to the literal unexpanded string if %LOCALAPPDATA% is not set.
func defaultGFNPath() string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return `%LOCALAPPDATA%\NVIDIA Corporation\GeForceNOW\CEF\GeForceNOW.exe`
	}
	return filepath.Join(local, `NVIDIA Corporation`, `GeForceNOW`, `CEF`, `GeForceNOW.exe`)
}

// containsString returns true if slice contains s (case-sensitive).
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
