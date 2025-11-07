package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/csnewman/team-cli/internal/team"
	"github.com/spf13/cobra"
)

func listAccountsCmdRun(cmd *cobra.Command, args []string) error {
	cfg, err := readConfigReAuth(cmd.Context())
	if err != nil {
		return fmt.Errorf("could not read config and authenticate: %w", err)
	}

	accounts, err := team.FetchAccounts(cmd.Context(), cfg.ServerConfig, cfg.AuthToken)
	if err != nil {
		return fmt.Errorf("could not fetch accounts: %w", err)
	}

	sorted := slices.SortedFunc(maps.Values(accounts), func(a *team.Account, b *team.Account) int {
		return strings.Compare(a.Name, b.Name)
	})

	fmt.Println()
	fmt.Println("Accounts:")

	for i, account := range sorted {
		fmt.Printf("  [%d] id=%q name=%q\n", i+1, account.ID, account.Name)

		slices.SortFunc(account.Permissions, func(a, b *team.Permission) int {
			return strings.Compare(a.Name, b.Name)
		})

		for _, permission := range account.Permissions {
			fmt.Printf("    - role=%q max_duration=%d requires_approval=%v\n", permission.Name, permission.MaxDuration, permission.RequiresApproval)
		}
	}

	return nil
}
