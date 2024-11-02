package mock_client

import "github.com/coder/websocket"

type mockClient struct {
	conn      *websocket.Conn
	send      chan []byte
	connID    string
	UserID    int32   `json:"userId,omitempty"`
	DiscordID *string `json:"discordId,omitempty"`
}

func New(conn_id string, user_id int32, discord_id string) *mockClient {
	return &mockClient{
		conn:      nil,
		send:      make(chan []byte, 1024),
		connID:    conn_id,
		UserID:    user_id,
		DiscordID: &discord_id,
	}
}

func (c *mockClient) Conn() *websocket.Conn {
	return c.conn
}

func (c *mockClient) ConnID() string {
	return c.connID
}

func (c *mockClient) ID() int32 {
	return c.UserID
}

func (c *mockClient) Send() chan []byte {
	return c.send
}
