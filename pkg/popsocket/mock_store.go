package popsocket

import "context"

type MockSessionStore struct {
	FindFunc              func(ctx context.Context, sid string) (Session, error)
	HasExpiredFunc        func(ctx context.Context, sid string) (bool, error)
	UserFromDiscordIDFunc func(ctx context.Context, discordID string) (int32, error)
}

func (m *MockSessionStore) Find(ctx context.Context, sid string) (Session, error) {
	return m.FindFunc(ctx, sid)
}

func (m *MockSessionStore) HasExpired(ctx context.Context, sid string) (bool, error) {
	return m.HasExpiredFunc(ctx, sid)
}

func (m *MockSessionStore) UserFromDiscordID(ctx context.Context, discordID string) (int32, error) {
	return m.UserFromDiscordIDFunc(ctx, discordID)
}
