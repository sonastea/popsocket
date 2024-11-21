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
	Send() chan []byte
}

type Client struct {
	conn *websocket.Conn
	send chan []byte

	connID    string
	UserID    int32   `json:"userId,omitempty"`
	DiscordID *string `json:"discordId,omitempty"`
	SID       string  `json:"sid"`
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

func (c *Client) Send() chan []byte {
	return c.send
}

// newClient returns an instance of Client with the corresponding user metadata.
func newClient(ctx context.Context, connKey string, conn *websocket.Conn) *Client {
	client := &Client{
		send:   make(chan []byte, MaxMessageSize),
		conn:   conn,
		connID: connKey,
	}

	if userID, ok := ctx.Value(USER_ID_KEY).(int32); ok {
		client.UserID = userID
	}

	if discordID, ok := ctx.Value(DISCORD_ID_KEY).(string); ok {
		client.DiscordID = &discordID
	}

	if SID, ok := ctx.Value(SID_KEY).(string); ok {
		client.SID = SID
	}

	return client
}

// messageReceiver listens for incoming messages from the client
// and processes them based on the message type.
func (p *PopSocket) messageReceiver(ctx context.Context, client *Client, cancel context.CancelCauseFunc) {
	defer func() {
		cancel(nil)
		p.LogInfo(fmt.Sprintf("Disconnected, context done for conn %s: client %d.", client.connID, client.ID()))
		p.unregister <- client
		client.Conn().Close(websocket.StatusNormalClosure, "Client disconnected")
	}()

	for {
		_, message, err := client.Conn().Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				p.LogInfo("Context was canceled, exiting messageReceiver.")
				client.Conn().Close(websocket.StatusNormalClosure, context.Cause(ctx).Error())
				return
			}

			closeStatus := websocket.CloseStatus(err)
			switch {
			case closeStatus == websocket.StatusNormalClosure || closeStatus == websocket.StatusGoingAway:
				return

			case closeStatus != -1:
				p.LogWarn(fmt.Sprintf("WebSocket closed for client %v, err: %+v", client.ID(), err))
				client.Conn().Close(websocket.StatusNormalClosure, "Receiver error or context done.")
				return

			default:
				p.LogWarn(fmt.Sprintf("Error reading from client %v: %v", client.ID(), err))
				return
			}
		}

		p.handleMessages(ctx, client, message)
	}
}

// messageSender dispatches messages from the PopSocket server to the associated client.
func (p *PopSocket) messageSender(ctx context.Context, client *Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-client.Send():
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, writeTimeout-10)
			defer cancel()
			if err := client.Conn().Write(writeCtx, websocket.MessageBinary, message); err != nil {
				p.LogError("messageSender write error: %w", err)
				return
			}
		}
	}
}
