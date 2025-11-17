package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/csnewman/team-cli/internal/team"
	"github.com/spf13/cobra"
)

var ErrInvalid = errors.New("invalid")

func requestCmdRun(cmd *cobra.Command, args []string) error {
	account, err := cmd.Flags().GetString("account")
	if err != nil {
		return fmt.Errorf("account flag: %w", err)
	}

	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return fmt.Errorf("role flag: %w", err)
	}

	start, err := cmd.Flags().GetString("start")
	if err != nil {
		return fmt.Errorf("start flag: %w", err)
	}

	duration, err := cmd.Flags().GetInt("duration")
	if err != nil {
		return fmt.Errorf("duration flag: %w", err)
	}

	ticket, err := cmd.Flags().GetString("ticket")
	if err != nil {
		return fmt.Errorf("ticket flag: %w", err)
	}

	reason, err := cmd.Flags().GetString("reason")
	if err != nil {
		return fmt.Errorf("reason flag: %w", err)
	}

	autoConfirm, err := cmd.Flags().GetBool("confirm")
	if err != nil {
		return fmt.Errorf("confirm flag: %w", err)
	}

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

	// Select account
	if len(sorted) == 0 {
		return fmt.Errorf("%w: no accounts found", ErrInvalid)
	}

	var selectedAccount *team.Account

	if account == "" {
		fmt.Println()
		fmt.Println("Please select the account:")
		for i, acc := range sorted {
			fmt.Printf("  [%d] id=%q name=%q\n", i+1, acc.ID, acc.Name)
		}

		fmt.Println()

		idx, err := promptSelection("Account option? ", 1, len(sorted))
		if err != nil {
			return fmt.Errorf("could not select account: %w", err)
		}

		selectedAccount = sorted[idx-1]
	} else {
		for _, acc := range accounts {
			if strings.EqualFold(acc.ID, account) || strings.EqualFold(acc.Name, account) {
				selectedAccount = acc

				break
			}
		}

		if selectedAccount == nil {
			return fmt.Errorf("%w: account %q not found", ErrInvalid, account)
		}
	}

	// Select role
	var selectedRole *team.Role

	allowedRoles := slices.SortedFunc(maps.Values(selectedAccount.Roles), func(a *team.Role, b *team.Role) int {
		return strings.Compare(a.Name, b.Name)
	})

	if role == "" {
		fmt.Println()
		fmt.Println("Please select the role:")
		for i, r := range allowedRoles {
			fmt.Printf(
				"  [%d] name=%q max_duration_with_approval=%d max_duration_without_approval=%d\n",
				i+1,
				r.Name,
				r.MaxDurApproval,
				r.MaxDurNoApproval,
			)
		}

		fmt.Println()

		idx, err := promptSelection("Role option? ", 1, len(sorted))
		if err != nil {
			return fmt.Errorf("could not select role: %w", err)
		}

		selectedRole = allowedRoles[idx-1]
	} else {
		for _, perm := range allowedRoles {
			if strings.EqualFold(perm.ID, role) || strings.EqualFold(perm.Name, role) {
				selectedRole = perm

				break
			}
		}

		if selectedRole == nil {
			return fmt.Errorf("%w: role %q not found", ErrInvalid, role)
		}
	}

	var startTime time.Time

	if start == "" {
		startTime, err = promptTime("Start time (e.g. 2006-01-02 15:04:05)? [now] ")
		if err != nil {
			return fmt.Errorf("could not select time: %w", err)
		}
	} else if !strings.EqualFold(start, "now") {
		startTime, err = time.ParseInLocation(time.DateTime, start, time.Local)
		if err != nil {
			return fmt.Errorf("could not parse start time: %w", err)
		}
	}

	if duration == 0 {
		duration, err = promptSelection(
			fmt.Sprintf("Duration (1-%d hours)? ", selectedRole.MaxDurApproval),
			1, selectedRole.MaxDurApproval,
		)
		if err != nil {
			return fmt.Errorf("could not select duration: %w", err)
		}
	} else if duration < 1 || duration > selectedRole.MaxDurApproval {
		return fmt.Errorf("%w: duration must be between 1 and %d", ErrInvalid, duration)
	}

	if ticket == "" {
		for {
			ticket, err = promptString("Ticket: ")
			if err != nil {
				return fmt.Errorf("could not select ticket: %w", err)
			}

			if team.TicketRegex.MatchString(ticket) {
				break
			}

			fmt.Println("Ticket format is not valid")
		}
	} else if !team.TicketRegex.MatchString(ticket) {
		return fmt.Errorf("%w: ticket format is no valid", ErrInvalid)
	}

	if reason == "" {
		reason, err = promptString("Justification: ")
		if err != nil {
			return fmt.Errorf("could not select justification: %w", err)
		}
	}

	fmt.Println("")
	fmt.Println("Details:")
	fmt.Printf("  Account: id=%q name=%q\n", selectedAccount.ID, selectedAccount.Name)
	fmt.Printf("  Role: name=%q\n", selectedRole.Name)

	if startTime.IsZero() {
		fmt.Println("  Start: now")
	} else {
		fmt.Printf("  Start: %q\n", startTime)
	}

	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Requires approval: %v\n", duration > selectedRole.MaxDurNoApproval)

	fmt.Printf("  Ticket: %q\n", ticket)
	fmt.Printf("  Justification: %q\n", reason)

	fmt.Println()

	if !autoConfirm {
		cont, err := promptBool("Confirm (y/n)? ")
		if err != nil {
			return fmt.Errorf("could not select confirmation: %w", err)
		}

		if !cont {
			return fmt.Errorf("%w: confirmation rejected", ErrInvalid)
		}
	}

	id, err := team.Request(cmd.Context(), cfg.ServerConfig, cfg.AuthToken, &team.AccessRequest{
		AccountID:     selectedAccount.ID,
		AccountName:   selectedAccount.Name,
		Role:          selectedRole.Name,
		RoleID:        selectedRole.ID,
		Duration:      duration,
		StartTime:     startTime,
		Justification: reason,
		Ticket:        ticket,
	})
	if err != nil {
		return fmt.Errorf("could not request role: %w", err)
	}

	fmt.Println("Request submitted")
	fmt.Printf("Request ID: %s\n", id)

	return nil
}
