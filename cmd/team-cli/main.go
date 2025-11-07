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

	requestCmd := &cobra.Command{
		Use:   "request",
		Short: "Request elevated access",
		Long: `Request temporary elevated access to a AWS account.

Exclude flags to perform interactive selection.`,
		Args: cobra.ExactArgs(0),
		RunE: requestCmdRun,
	}

	requestCmd.Flags().StringP("account", "a", "", "AWS account ID or name")
	requestCmd.Flags().StringP("role", "r", "", "AWS role ID or name")
	requestCmd.Flags().StringP("start", "s", "", "Start date and time")
	requestCmd.Flags().IntP("duration", "d", 0, "Duration of elevation")
	requestCmd.Flags().StringP("ticket", "t", "", "Ticket ID")
	requestCmd.Flags().StringP("reason", "j", "", "Justification reason")
	requestCmd.Flags().BoolP("confirm", "y", false, "Automatically confirm")

	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(listAccountsCmd)
	rootCmd.AddCommand(requestCmd)
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
