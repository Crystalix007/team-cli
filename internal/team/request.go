package team

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	"github.com/csnewman/team-cli/internal/gql"
)

var TicketRegex = regexp.MustCompile("^[a-zA-Z0-9_-]+$")

const createRequest = `mutation CreateRequests(
    $input: CreateRequestsInput!
    $condition: ModelRequestsConditionInput
  ) {
    createRequests(input: $input, condition: $condition) {
      id
      email
      accountId
      accountName
      role
      roleId
      startTime
      duration
      justification
      status
      comment
      username
      approver
      approverId
      approvers
      approver_ids
      revoker
      revokerId
      endTime
      ticketNo
      revokeComment
      session_duration
      createdAt
      updatedAt
      owner
      __typename
    }
  }`

type AccessRequest struct {
	AccountID     string
	AccountName   string
	Role          string
	RoleID        string
	Duration      int
	StartTime     time.Time
	Justification string
	Ticket        string
}

type rawCreateRequestResponse struct {
	CreateRequests struct {
		Id string `json:"id"`
	} `json:"createRequests"`
}

func Request(ctx context.Context, remote *RemoteConfig, token *AuthToken, req *AccessRequest) (string, error) {
	slog.Info("Requesting access")

	startTime := req.StartTime

	if startTime.IsZero() {
		startTime = time.Now()
	}

	startTime = startTime.Truncate(time.Minute)

	resp, err := gql.Execute(ctx, remote.GraphQLEndpoint, token.AccessToken, &gql.Request{
		Query: createRequest,
		Variables: map[string]any{
			"input": map[string]any{
				"accountId":     req.AccountID,
				"accountName":   req.AccountName,
				"role":          req.Role,
				"roleId":        req.RoleID,
				"duration":      strconv.Itoa(req.Duration),
				"startTime":     startTime.UTC().Format(time.RFC3339),
				"justification": req.Justification,
				"ticketNo":      req.Ticket,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to execute: %w", err)
	}

	if len(resp.Errors) > 0 {
		for _, err := range resp.Errors {
			slog.Error("Received error from server", "error", err)
		}

		return "", fmt.Errorf("%w: server returned an error", ErrUnexpected)
	}

	var rawResult rawCreateRequestResponse

	if err := resp.UnmarshalData(&rawResult); err != nil {
		return "", fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return rawResult.CreateRequests.Id, nil
}
