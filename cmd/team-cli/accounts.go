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

	sortedAccs := slices.SortedFunc(maps.Values(accounts), func(a *team.Account, b *team.Account) int {
		return strings.Compare(a.Name, b.Name)
	})

	fmt.Println()
	fmt.Println("Accounts:")

	for i, account := range sortedAccs {
		fmt.Printf("  [%d] id=%q name=%q\n", i+1, account.ID, account.Name)

		roles := slices.SortedFunc(maps.Values(account.Roles), func(a *team.Role, b *team.Role) int {
			return strings.Compare(a.Name, b.Name)
		})

		for _, role := range roles {
			fmt.Printf(
				"    - role=%q max_duration_with_approval=%d max_duration_without_approval=%d\n",
				role.Name,
				role.MaxDurApproval,
				role.MaxDurNoApproval,
			)
		}
	}

	return nil
}
