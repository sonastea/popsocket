package popsocket

import "github.com/coder/websocket"

type client interface {
	Conn() *websocket.Conn
	ID() int
}

type Client struct {
	connID    string
	UserID    *int    `json:"userId,omitempty"`
	DiscordID *string `json:"discordId,omitempty"`
	conn      *websocket.Conn
}

func (c *Client) Conn() *websocket.Conn {
	return c.conn
}

func (c *Client) ID() int {
	return *c.UserID
}
