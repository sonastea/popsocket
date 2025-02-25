package popsocket

import (
	"context"
	"testing"
)

func TestClientID(t *testing.T) {
	client := &Client{UserID: 9}

	if client.ID() != 9 {
		t.Errorf("Expected client.ID() to return 9, got %d", client.ID())
	}
}

func TestNewClient(t *testing.T) {
	var (
		discordID = "9"
		sid       = "sid_key"
		userID    = int32(9)
	)

	t.Run("UserID, DiscordID, & SID Present", func(t *testing.T) {
		ctx := context.Background()
		ctx = context.WithValue(ctx, USER_ID_KEY, userID)
		ctx = context.WithValue(ctx, DISCORD_ID_KEY, discordID)
		ctx = context.WithValue(ctx, SID_KEY, sid)

		client := newClient(ctx, "connId=9", nil)

		if client.ID() != userID {
			t.Errorf("Expected client's UserID = 9, got %d", client.ID())
		}

		if *client.DiscordID != discordID {
			t.Errorf("Expected client's DiscordID = %s, got %s", discordID, *client.DiscordID)
		}

		if client.SID != sid {
			t.Errorf("Expected client's SID = %s, got %s", sid, client.SID)
		}

		if client.send == nil {
			t.Error("Expected client's send channel to not be nil")
		}
	})

	t.Run("UserID Present", func(t *testing.T) {
		ctx := context.Background()
		ctx = context.WithValue(ctx, USER_ID_KEY, userID)

		client := newClient(ctx, "connId=9", nil)

		if client.ID() != userID {
			t.Errorf("Expected client's UserID = 9, got %d", client.ID())
		}

		if client.DiscordID != nil {
			t.Errorf("Expected client's DiscordID to be nil, got %v", *client.DiscordID)
		}
	})

	t.Run("DiscordID Present", func(t *testing.T) {
		ctx := context.Background()
		ctx = context.WithValue(ctx, DISCORD_ID_KEY, discordID)

		client := newClient(ctx, "connId=9", nil)

		if client.ID() != 0 {
			t.Errorf("Expected client's UserID = 0, got %d", client.ID())
		}

		if *client.DiscordID != discordID {
			t.Errorf("Expected client's DiscordID = %s, got %s", discordID, *client.DiscordID)
		}
	})
}
