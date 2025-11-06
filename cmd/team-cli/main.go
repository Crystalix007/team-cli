package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "team-cli",
		Short: "AWS TEAM CLI interface",
		Long:  `team-cli is a CLI wrapper for accessing AWS TEAM`,
	}

	configureCmd := &cobra.Command{
		Use:   "configure [server]",
		Short: "Configure AWS TEAM",
		Long:  `Configure the AWS TEAM server to connect to`,
		Args:  cobra.ExactArgs(1),
		RunE:  configureCmdRun,
	}

	rootCmd.AddCommand(configureCmd)
	rootCmd.SilenceUsage = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
