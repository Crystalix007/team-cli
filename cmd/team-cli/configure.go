package main

import (
	"log/slog"

	"github.com/csnewman/team-cli/internal/team"
	"github.com/spf13/cobra"
)

func configureCmdRun(cmd *cobra.Command, args []string) error {
	remoteCfg, err := team.ExtractConfig(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	slog.Info("Extracted remote configuration", "cfg", remoteCfg)

	return nil
}
