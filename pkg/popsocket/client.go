package popsocket

import (
	"context"
	"encoding/json"
	"fmt"

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
func (p *PopSocket) messageReceiver(ctx context.Context, client *Client, cancel context.CancelFunc) {
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			p.LogInfo("Context done, message receiver stopping")
			return

		default:
			_, msg, err := client.Conn().Read(ctx)
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

			var rawMap map[string]json.RawMessage
			err = json.Unmarshal(msg, &rawMap)
			if err != nil {
				p.LogWarn(fmt.Sprintf("Unable to unmarshal message by client %v, got %s", client, string(msg)))
				continue
			}

			if _, ok := rawMap["event"]; ok {
				var recv EventMessage
				err = json.Unmarshal(rawMap["event"], &recv.Event)
				if err != nil {
					p.LogWarn(fmt.Sprintf("Unable to unmarshal message by client %v, got %+v. Error: %v", client.ID(), rawMap, err))
				}

				switch recv.Event.Type() {
				case EventMessageType.Connect.Type():
					json.Unmarshal(msg, &recv.Content)
					msg, err := json.Marshal(&EventMessage{Event: EventMessageType.Connect, Content: "pong!"})
					if err != nil {
						p.LogError("Error marshalling connect message: %w", err)
					}
					err = client.Conn().Write(ctx, websocket.MessageText, msg)

				case EventMessageType.Conversations.Type():
					json.Unmarshal(msg, &recv.Content)
					conversations, err := p.messageStore.Convos(ctx, client.ID())
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

				case EventMessageType.MarkAsRead.Type():
          var content ContentMarkAsRead
          json.Unmarshal(rawMap["content"], &content)

					p.LogWarn(fmt.Sprintf("MarkAsRead %+v", content))

				case EventMessageType.SetRecipient.Type():

				default:
					p.LogWarn(fmt.Sprintf("received: %+v", recv))
				}

			} else if _, ok := rawMap["convId"]; ok {
				p.LogWarn(fmt.Sprintf("message received: %+v", msg))
			}
		}
	}
}

// messageSender dispatches messages from the PopSocket to the associated client.
func (p *PopSocket) messageSender(ctx context.Context, client *Client) {
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}
