package popsocket

import (
	"context"
	"fmt"

	"github.com/coder/websocket"
)

type client interface {
	Conn() *websocket.Conn
	ConnID() string
	ID() int32
}

type Client struct {
	conn *websocket.Conn
	send chan []byte

	connID    string
	UserID    int32   `json:"userId,omitempty"`
	DiscordID *string `json:"discordId,omitempty"`
}

func (c *Client) Conn() *websocket.Conn {
	return c.conn
}

func (c *Client) ConnID() string {
	return c.connID
}

func (c *Client) ID() int32 {
	return c.UserID
}

// newClient returns an instance of Client with the corresponding user metadata.
func newClient(ctx context.Context, connKey string, conn *websocket.Conn) *Client {
	client := &Client{
		send:   make(chan []byte),
		conn:   conn,
		connID: connKey,
	}

	if userID, ok := ctx.Value(USER_ID_KEY).(int32); ok {
		client.UserID = userID
	}

	if discordID, ok := ctx.Value(DISCORD_ID_KEY).(string); ok {
		client.DiscordID = &discordID
	}

	return client
}

// messageReceiver listens for incoming messages from the client
// and processes them based on the message type.
func (p *PopSocket) messageReceiver(ctx context.Context, client *Client, cancel context.CancelFunc) {
	defer func() {
		cancel()
		client.Conn().Close(websocket.StatusNormalClosure, "Client disconnected")
	}()

	for {
		select {
		case <-ctx.Done():
			p.LogInfo("Context done, message receiver stopping")
			return

		default:
			_, message, err := client.Conn().Read(ctx)
			if err != nil {
				// Handle contextion cancellation explicitly
				if ctx.Err() != nil {
					p.LogInfo("Context was canceled, exiting messageReceiver.")
					client.Conn().Close(websocket.StatusNormalClosure, "Receiver error or context done.")
					return
				}

				closeStatus := websocket.CloseStatus(err)
				if closeStatus != -1 {
					if closeStatus == websocket.StatusGoingAway || closeStatus == websocket.StatusNoStatusRcvd {
						return
					}

					p.LogWarn(fmt.Sprintf("WebSocket closed for client %v, err: %+v", client.ID(), err))
					client.Conn().Close(websocket.StatusNormalClosure, "Receiver error or context done.")
					return
				}

				p.LogWarn(fmt.Sprintf("Error reading from client %v: %v", client.ID(), err))
				return
			}

			go p.handleMessages(ctx, client, message)
		}
	}
}

// messageSender dispatches messages from the PopSocket server to the associated client.
func (p *PopSocket) messageSender(ctx context.Context, client *Client) {
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}
