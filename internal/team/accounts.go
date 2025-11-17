package team

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/csnewman/team-cli/internal/gql"
)

const (
	policySubscription = `subscription OnPublishPolicy {
    onPublishPolicy {
      id
      policy {
        accounts {
          name
          id
          __typename
        }
        permissions {
          name
          id
          __typename
        }
        approvalRequired
        duration
        __typename
      }
      username
      __typename
    }
  }`
	policyRequest = `query GetUserPolicy($userId: String, $groupIds: [String]) {
  getUserPolicy(userId: $userId, groupIds: $groupIds) {
    id
    policy {
      accounts {
        name
        id
        __typename
      }
      permissions {
        name
        id
        __typename
      }
      approvalRequired
      duration
      __typename
    }
    username
    __typename
  }
}`
)

type rawPolicyData struct {
	OnPublishPolicy struct {
		Id     string `json:"id"`
		Policy []struct {
			Accounts []struct {
				Name     string `json:"name"`
				Id       string `json:"id"`
				Typename string `json:"__typename"`
			} `json:"accounts"`
			Permissions []struct {
				Name     string `json:"name"`
				Id       string `json:"id"`
				Typename string `json:"__typename"`
			} `json:"permissions"`
			ApprovalRequired bool   `json:"approvalRequired"`
			Duration         string `json:"duration"`
			Typename         string `json:"__typename"`
		} `json:"policy"`
		Username string `json:"username"`
		Typename string `json:"__typename"`
	} `json:"onPublishPolicy"`
}

type Account struct {
	ID    string
	Name  string
	Roles map[string]*Role
}

type Role struct {
	ID   string
	Name string

	MaxDurNoApproval int
	MaxDurApproval   int
}

func FetchAccounts(ctx context.Context, remote *RemoteConfig, token *AuthToken) (map[string]*Account, error) {
	slog.Info("Fetching AWS accounts")

	idTok, err := token.ParseIDToken()
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID token: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	var rawPolicy rawPolicyData

	if err := gql.Subscribe(
		ctx,
		remote.GraphQLEndpoint,
		token.AccessToken,
		&gql.Request{
			Query: policySubscription,
		},
		func(ctx context.Context) error {
			if _, err := gql.Execute(ctx, remote.GraphQLEndpoint, token.AccessToken, &gql.Request{
				Query: policyRequest,
				Variables: map[string]any{
					"userId":   idTok.UserID,
					"groupIds": strings.Split(idTok.GroupIDs, ","),
				},
			}); err != nil {
				return fmt.Errorf("failed to request: %w", err)
			}

			return nil
		},
		func(ctx context.Context, payload *gql.Payload) (bool, error) {
			if err := payload.UnmarshalData(&rawPolicy); err != nil {
				return false, fmt.Errorf("failed to unmarshal payload: %w", err)
			}

			return false, nil
		},
	); err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	accounts := make(map[string]*Account)

	for _, pol := range rawPolicy.OnPublishPolicy.Policy {
		slog.Debug("Policy", "dur", pol.Duration, "approval_required", pol.ApprovalRequired)

		duration, err := strconv.Atoi(pol.Duration)
		if err != nil {
			return nil, fmt.Errorf("failed to parse policy duration %q: %w", pol.Duration, err)
		}

		for _, account := range pol.Accounts {
			slog.Debug("Account", "name", account.Name, "id", account.Id)

			acc, ok := accounts[account.Id]
			if !ok {
				acc = &Account{
					ID:    account.Id,
					Name:  account.Name,
					Roles: make(map[string]*Role),
				}

				accounts[account.Id] = acc
			}

			for _, perm := range pol.Permissions {
				slog.Debug("Permission", "name", perm.Name, "id", perm.Id)

				role, ok := acc.Roles[perm.Id]
				if !ok {
					role = &Role{
						ID:   perm.Id,
						Name: perm.Name,
					}

					acc.Roles[perm.Id] = role
				}

				role.MaxDurApproval = max(duration, role.MaxDurApproval)

				if !pol.ApprovalRequired {
					role.MaxDurNoApproval = max(duration, role.MaxDurNoApproval)
				}
			}
		}
	}

	return accounts, nil
}
