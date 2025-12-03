package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var Version = "(unknown version)"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		Version = info.Main.Version
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:               "team-cli",
		Short:             "AWS TEAM CLI interface",
		Long:              "Team-CLI - " + Version + "\n\nteam-cli is a CLI wrapper for accessing AWS TEAM.",
		Version:           Version,
		PersistentPreRunE: rootCmdPersistentPre,
	}

	rootCmd.PersistentFlags().CountP("verbose", "v", "increase verbosity")

	configureCmd := &cobra.Command{
		Use:   "configure [server]",
		Short: "Configure AWS TEAM",
		Long:  `Configure the AWS TEAM server to connect to`,
		Args:  cobra.ExactArgs(1),
		RunE:  configureCmdRun,
	}

	configureCmd.Flags().BoolP("no-browser", "b", false, "Do not open the browser automatically")
	configureCmd.Flags().BoolP("device-code", "d", false, "Use the device code flow. Implies --no-browser")

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

	approveCmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve elevated access",
		Long: `Approve temporary elevated access to a AWS account.

Exclude flags to perform interactive selection.`,
		Args: cobra.ExactArgs(0),
		RunE: approveCmdRun,
	}

	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(listAccountsCmd)
	rootCmd.AddCommand(requestCmd)
	rootCmd.AddCommand(approveCmd)
	rootCmd.SilenceUsage = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func rootCmdPersistentPre(cmd *cobra.Command, _ []string) error {
	verbose, err := cmd.Flags().GetCount("verbose")
	if err != nil {
		return fmt.Errorf("could not get verbose flag: %w", err)
	}

	level := slog.LevelWarn

	if verbose > 1 {
		level = slog.LevelDebug
	} else if verbose > 0 {
		level = slog.LevelInfo
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource:   false,
		Level:       level,
		ReplaceAttr: nil,
	})))

	fmt.Println("# Team-CLI - " + Version)

	if strings.HasPrefix(Version, "v") {
		latestVersion, err := getLatestVersion(cmd.Context())
		if err != nil {
			slog.Warn("Failed to check for updates", "err", err)
		} else if !strings.HasPrefix(latestVersion, "v") {
			slog.Warn("Failed to check for updates", "version", latestVersion, "err", "unknown format")
		} else if semver.Compare(latestVersion, Version) > 0 {
			fmt.Println()
			fmt.Println("---- Update available! ----")
			fmt.Println("A new release is available. Please install with: go install github.com/csnewman/team-cli/cmd/team-cli@" + latestVersion)
		}
	}

	return nil
}

const latestURL = "https://api.github.com/repos/csnewman/team-cli/releases/latest"

var ErrUnexpected = errors.New("unexpected error")

func getLatestVersion(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: could not fetch: %v", ErrUnexpected, resp.Status)
	}

	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read response body: %w", err)
	}

	var versionBlob struct {
		TagName string `json:"tag_name"`
	}

	if err := json.Unmarshal(rawBody, &versionBlob); err != nil {
		return "", fmt.Errorf("could not unmarshal response body: %w", err)
	}

	return versionBlob.TagName, nil
}
