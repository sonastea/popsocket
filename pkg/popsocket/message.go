package popsocket

import (
	"context"
	"fmt"
	"time"

	"github.com/sonastea/popsocket/pkg/db"
)

type MessageStore interface {
	Convos(ctx context.Context, userID int) (*ConversationsResponse, error)
	UpdateAsRead(ctx context.Context, msg ContentMarkAsRead) (ContentMarkAsReadResponse, error)
}

type MessageService struct {
	messageStore MessageStore
}

type messageStore struct {
	db db.DB
}

// MessageInterface defines a contract for all message types.
type MessageInterface interface {
	Type() string
}

type messageEventType string

var EventMessageType = struct {
	Connect       messageEventType
	Conversations messageEventType
	MarkAsRead    messageEventType
	SetRecipient  messageEventType
}{
	Connect:       messageEventType("CONNECT"),
	Conversations: messageEventType("CONVERSATIONS"),
	MarkAsRead:    messageEventType("MARK_AS_READ"),
	SetRecipient:  messageEventType("SET_RECIPIENT"),
}

// EventMessage represents a message used for event-driven communication.
type EventMessage struct {
	Event   messageEventType `json:"event"`
	Content string           `json:"content"`
}

// ContentMarkAsRead is the struct representing content from a EventMessageType.MarkAsRead
type ContentMarkAsRead struct {
	Convid  string `json:"convid"`
	To      int    `json:"to"`
	Content string `json:"content"`
	Read    bool   `json:"read"`
}

// ContentMarkAsReadResponse is the struct representing content from a EventMessageType.MarkAsRead
type ContentMarkAsReadResponse struct {
	Convid string `json:"convid"`
	To     int    `json:"to"`
	Unread int    `json:"unread"`
	Read   bool   `json:"read"`
}

// Type returns the underlying type as a string.
func (m messageEventType) Type() string {
	return string(m)
}

// Type returns the type of the message to distinguish its role.
func (em *EventMessage) Type() string {
	return string(em.Event)
}

type ConversationsResponse struct {
	Conversations []Conversation `json:"conversations"`
}

type Message struct {
	Convid    string    `json:"convid"`
	To        int       `json:"to"`
	From      int       `json:"from"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	FromSelf  bool      `json:"fromSelf"`
	Read      bool      `json:"read"`
}

type Conversation struct {
	ID          int       `json:"id"`     // serial integer
	Convid      string    `json:"convid"` // unique identifier (uuid)
	Username    string    `json:"username"`
	Displayname string    `json:"displayname"`
	Photo       *string   `json:"photo"`
	Status      string    `json:"status"`
	Messages    []Message `json:"messages"`
	Unread      int       `json:"unread"`
}

// NewMessageService creates a new instance of MessageService.
func NewMessageService(store MessageStore) MessageService {
	return MessageService{
		store,
	}
}

// NewMessageStore creates a new instance of sessionStore.
func NewMessageStore(db db.DB) *messageStore {
	return &messageStore{db: db}
}

func (m *Message) Type() string {
	return "Message"
}

func (ms *messageStore) Convos(ctx context.Context, userID int) (*ConversationsResponse, error) {
	query := `
        WITH user_conversations AS (
          SELECT DISTINCT c.id, c.convid
          FROM "Conversation" c
          JOIN "_ConversationToUser" cu ON c.id = cu."A"
          WHERE cu."B" = $1
        ),
        conversation_users AS (
          SELECT uc.id, uc.convid, u.id as user_id, u.username, u.displayname, u.photo, u.status
          FROM user_conversations uc
          JOIN "_ConversationToUser" cu ON uc.id = cu."A"
          JOIN "User" u ON cu."B" = u.id
          WHERE u.id != $1
        ),
        conversation_messages AS (
          SELECT c.id, c.convid, m.*
          FROM user_conversations c
          LEFT JOIN "Message" m ON c.convid = m."convId"
        )
        SELECT
          cm."recipientId" as id, cu.convid, cu.username, cu.displayname, cu.photo, cu.status,
          cm."recipientId", cm."userId", cm.content, cm."createdAt", cm."fromSelf", cm.read
        FROM conversation_users cu
        LEFT JOIN conversation_messages cm ON cu.convid = cm.convid
        ORDER BY cu.id, cm."createdAt" desc
        `

	rows, err := ms.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("Error querying conversations: %w", err)
	}
	defer rows.Close()

	conversationsMap := make(map[string]*Conversation)

	for rows.Next() {
		var conv Conversation
		var msg Message
		var recipientID, fromUserID *int

		err := rows.Scan(
			&conv.ID,
			&conv.Convid,
			&conv.Username,
			&conv.Displayname,
			&conv.Photo,
			&conv.Status,
			&recipientID,
			&fromUserID,
			&msg.Content,
			&msg.CreatedAt,
			&msg.FromSelf,
			&msg.Read,
		)
		if err != nil {
			return nil, fmt.Errorf("Error scanning row: %w", err)
		}

		existingConv, exists := conversationsMap[conv.Convid]
		if !exists {
			conv.Messages = []Message{}
			conv.Unread = 0
			conversationsMap[conv.Convid] = &conv
			existingConv = &conv
		}

		msg.Convid = conv.Convid
		if recipientID != nil {
			msg.To = *recipientID
		}
		if fromUserID != nil {
			msg.From = *fromUserID
		}
		existingConv.Messages = append(existingConv.Messages, msg)
		if !msg.Read && !msg.FromSelf {
			existingConv.Unread++
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Error iterating rows: %w", err)
	}

	result := &ConversationsResponse{
		Conversations: make([]Conversation, 0, len(conversationsMap)),
	}
	for _, conv := range conversationsMap {
		result.Conversations = append(result.Conversations, *conv)
	}

	return result, nil
}

func (ms *messageStore) UpdateAsRead(ctx context.Context, msg ContentMarkAsRead) (ContentMarkAsReadResponse, error) {
	query := `
    UPDATE "Message"
    SET read = true
    WHERE "convId" = $1 AND "recipientId" = $2 AND read = false
  `

	_, err := ms.db.Exec(ctx, query, msg.Convid, msg.To)
	if err != nil {
		return ContentMarkAsReadResponse{}, err
	}

	return ContentMarkAsReadResponse{
		Convid: msg.Convid,
		Unread: 0,
		To:     msg.To,
		Read:   true,
	}, nil
}
