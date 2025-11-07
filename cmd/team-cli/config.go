package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/csnewman/team-cli/internal/team"
)

var ErrInvalidConfig = errors.New("invalid config")

type Config struct {
	ServerConfig *team.RemoteConfig `json:"server_config"`
	AuthToken    *team.AuthToken    `json:"auth_token"`
}

func configPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user dir: %w", err)
	}

	teamPath := filepath.Join(homeDir, ".config", "team-cli")

	if err := os.MkdirAll(teamPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create team config dir: %w", err)
	}

	return filepath.Join(teamPath, "config.json"), nil
}

func readConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return new(Config), nil
		}

		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config *Config

	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	return config, nil
}

func writeConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	enc, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal config file: %w", err)
	}

	if err := os.WriteFile(path, enc, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func readConfigReAuth(ctx context.Context) (*Config, error) {
	cfg, err := readConfig()
	if err != nil {
		return nil, fmt.Errorf("could not read config: %w", err)
	}

	if cfg.ServerConfig == nil || cfg.ServerConfig.OAuthDomain == "" {
		slog.Error("No server config found!")

		return nil, ErrInvalidConfig
	}

	if cfg.AuthToken != nil && time.Now().Add(time.Minute*5).Before(cfg.AuthToken.ExpiresAt) {
		slog.Info("Existing auth token is valid")

		return cfg, nil
	}

	if cfg.AuthToken != nil && cfg.AuthToken.RefreshToken != "" {
		slog.Info("Existing auth token has expired, attempting to refresh")

		newToken, err := team.RefreshToken(ctx, cfg.ServerConfig, cfg.AuthToken)
		if err == nil {
			slog.Info("Refreshed token")

			cfg.AuthToken = newToken

			if err := writeConfig(cfg); err != nil {
				return nil, fmt.Errorf("failed to write new token: %w", err)
			}

			return cfg, nil
		}

		slog.Warn("Failed to refresh token", "err", err)
	}

	slog.Info("Reauthentication required")

	newToken, err := team.FetchToken(ctx, cfg.ServerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch new token: %w", err)
	}

	cfg.AuthToken = newToken

	if err := writeConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to write new token: %w", err)
	}

	return cfg, nil
}
