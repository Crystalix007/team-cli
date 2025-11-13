package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/csnewman/team-cli/internal/team"
	"github.com/spf13/cobra"
)

func configureCmdRun(cmd *cobra.Command, args []string) error {
	useDeviceCode, err := cmd.Flags().GetBool("device-code")
	if err != nil {
		return fmt.Errorf("device-code flag: %w", err)
	}

	noBrowser, err := cmd.Flags().GetBool("no-browser")
	if err != nil {
		return fmt.Errorf("no-browser flag: %w", err)
	}

	remoteCfg, err := team.ExtractConfig(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	slog.Info("Extracted remote configuration", "cfg", remoteCfg)

	var token *team.AuthToken

	if useDeviceCode {
		token, err = team.FetchTokenViaDeviceCode(cmd.Context(), remoteCfg, func(_ context.Context) (string, error) {
			return promptString("Device code? ")
		})
	} else {
		token, err = team.FetchToken(cmd.Context(), remoteCfg, noBrowser)
	}

	if err != nil {
		return err
	}

	slog.Info("Fetched initial token")

	existingCfg, err := readConfig()
	if err != nil {
		return fmt.Errorf("failed to read existing config: %w", err)
	}

	existingCfg.UseDeviceCode = useDeviceCode
	existingCfg.NoBrowser = noBrowser
	existingCfg.ServerConfig = remoteCfg
	existingCfg.AuthToken = token

	if err := writeConfig(existingCfg); err != nil {
		return fmt.Errorf("failed to write existing config: %w", err)
	}

	slog.Info("TEAM CLI config updated")

	return nil
}
