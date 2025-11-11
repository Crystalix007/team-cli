package team_test

import (
	"testing"

	"github.com/csnewman/team-cli/internal/team"
	"github.com/stretchr/testify/require"
)

func TestTicketRegex(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{
		"123", "ticket123", "ticket-123", "ticket_123",
	} {
		t.Run("valid="+valid, func(t *testing.T) {
			t.Parallel()

			require.True(t, team.TicketRegex.MatchString(valid))
		})
	}

	for _, invalid := range []string{
		" 123 ", "  ", "|||||",
	} {
		t.Run("invalid="+invalid, func(t *testing.T) {
			t.Parallel()

			require.False(t, team.TicketRegex.MatchString(invalid))
		})
	}
}
