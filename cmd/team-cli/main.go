package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:               "team-cli",
		Short:             "AWS TEAM CLI interface",
		Long:              `team-cli is a CLI wrapper for accessing AWS TEAM`,
		PersistentPreRunE: rootCmdPersistentPre,
	}

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")

	configureCmd := &cobra.Command{
		Use:   "configure [server]",
		Short: "Configure AWS TEAM",
		Long:  `Configure the AWS TEAM server to connect to`,
		Args:  cobra.ExactArgs(1),
		RunE:  configureCmdRun,
	}

	listAccountsCmd := &cobra.Command{
		Use:   "list-accounts",
		Short: "List all accounts",
		Long:  `List all AWS accounts you can use to access via AWS TEAM`,
		Args:  cobra.ExactArgs(0),
		RunE:  listAccountsCmdRun,
	}

	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(listAccountsCmd)
	rootCmd.SilenceUsage = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func rootCmdPersistentPre(cmd *cobra.Command, _ []string) error {
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return fmt.Errorf("could not get verbose flag: %w", err)
	}

	level := slog.LevelInfo

	if verbose {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource:   false,
		Level:       level,
		ReplaceAttr: nil,
	})))

	return nil
}
