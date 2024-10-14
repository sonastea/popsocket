package popsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/coder/websocket"
)

type client interface {
	Conn() *websocket.Conn
	ID() int
}

type Client struct {
	connID    string
	UserID    int     `json:"userId,omitempty"`
	DiscordID *string `json:"discordId,omitempty"`
	conn      *websocket.Conn
}

func (c *Client) Conn() *websocket.Conn {
	return c.conn
}

func (c *Client) ID() int {
	return c.UserID
}

// newClient returns an instance of Client with the corresponding user metadata.
func newClient(ctx context.Context, connKey string, conn *websocket.Conn) *Client {
	client := &Client{
		connID: connKey,
		conn:   conn,
	}

	if userID, ok := ctx.Value(USER_ID_KEY).(int); ok {
		client.UserID = userID
	}

	if discordID, ok := ctx.Value(DISCORD_ID_KEY).(string); ok {
		client.DiscordID = &discordID
	}

	return client
}

// messageReceiver listens for incoming messages from the client
// and processes them based on the message type.
func (p *PopSocket) messageReceiver(ctx context.Context, client client) {
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("message receiver done")
			return

		default:
			_, msg, err := client.Conn().Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if websocket.CloseStatus(err) != -1 {
					p.LogWarn(fmt.Sprintf("WebSocket closed for client %v: %v", client, err))
					return
				}
				p.LogWarn(fmt.Sprintf("Error reading from client %v: %v", client, err))
				return
			}

			var recv EventMessage
			err = json.Unmarshal(msg, &recv)
			if err != nil {
				p.LogWarn(fmt.Sprintf("Unable to unmarshal message by client %v, got %s", client, string(msg)))
				continue
			}

			switch recv.Event {
			case EventMessageType.Connect:
				msg, err := json.Marshal(&EventMessage{Event: EventMessageType.Connect, Content: "pong!"})
				if err != nil {
					p.LogError("Error marshalling connect message: %w", err)
				}

				err = client.Conn().Write(ctx, websocket.MessageText, msg)

			case EventMessageType.Conversations:
				conversations, err := p.MessageStore.Convos(ctx, client.ID())
				if err != nil {
					p.LogError("Error getting client's conversations: %w", err)
				}
				convos, err := json.Marshal(conversations.Conversations)
				if err != nil {
					p.LogError("Error marshalling conversations: %w", err)
				}

				msg, _ := json.Marshal(EventMessage{
					Event:   EventMessageType.Conversations,
					Content: fmt.Sprintf(`["%s", %s]`, EventMessageType.Conversations, string(convos)),
				})

				err = client.Conn().Write(ctx, websocket.MessageText, msg)

			default:
				p.LogInfo(fmt.Sprintf("received: %+v", recv))

			}
		}
	}
}

// messageSender dispatches messages from the PopSocket to the associated client.
func (p *PopSocket) messageSender(ctx context.Context, client *Client) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := client.conn.Write(ctx, websocket.MessageText, []byte(strconv.Itoa(client.ID())))
			if err != nil {
				if ctx.Err() != nil {
					p.LogWarn(fmt.Sprintf("Error writing to client %d: %v", client.ID(), err))
					return
				}
			}
		}
	}
}
