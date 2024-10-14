package popsocket

import "context"

type MockSessionStore struct {
	FindFunc              func(ctx context.Context, sid string) (Session, error)
	UserFromDiscordIDFunc func(ctx context.Context, discordID string) (int, error)
}

func (m *MockSessionStore) Find(ctx context.Context, sid string) (Session, error) {
	return m.FindFunc(ctx, sid)
}

func (m *MockSessionStore) UserFromDiscordID(ctx context.Context, discordID string) (int, error) {
	return m.UserFromDiscordIDFunc(ctx, discordID)
}
