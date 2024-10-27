package popsocket

import (
	"context"
	"fmt"
	"time"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	"github.com/sonastea/popsocket/pkg/db"
)

type MessageStore interface {
	Convos(ctx context.Context, user_id int32) (*ipc.ContentConversationsResponse, error)
	UpdateAsRead(ctx context.Context, msg *ipc.ContentMarkAsRead) (*ipc.ContentMarkAsReadResponse, error)
}

type MessageService struct {
	messageStore MessageStore
}

type messageStore struct {
	db db.DB
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

func (ms *messageStore) Convos(ctx context.Context, user_id int32) (*ipc.ContentConversationsResponse, error) {
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

	rows, err := ms.db.Query(ctx, query, user_id)
	if err != nil {
		return nil, fmt.Errorf("Error querying conversations: %w", err)
	}
	defer rows.Close()

	conversationsMap := make(map[string]*ipc.Conversation)

	for rows.Next() {
		var conv ipc.Conversation
		var msg ipc.Message
		var recipientID, fromUserID *int32
		var createdAt time.Time

		err := rows.Scan(
			&conv.Id,
			&conv.Convid,
			&conv.Username,
			&conv.Displayname,
			&conv.Photo,
			&conv.Status,
			&recipientID,
			&fromUserID,
			&msg.Content,
			&createdAt,
			&msg.FromSelf,
			&msg.Read,
		)
		if err != nil {
			return nil, fmt.Errorf("Error scanning row: %w", err)
		}

		existingConv, exists := conversationsMap[conv.Convid]
		if !exists {
			conv.Messages = []*ipc.Message{}
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
		msg.CreatedAt = createdAt.Format(time.RFC3339)
		existingConv.Messages = append(existingConv.Messages, &msg)
		if !msg.Read && !msg.FromSelf {
			existingConv.Unread++
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Error iterating rows: %w", err)
	}

	result := &ipc.ContentConversationsResponse{
		Conversations: make([]*ipc.Conversation, 0, len(conversationsMap)),
	}
	for _, conv := range conversationsMap {
		result.Conversations = append(result.Conversations, conv)
	}

	return result, nil
}

func (ms *messageStore) UpdateAsRead(ctx context.Context, msg *ipc.ContentMarkAsRead) (*ipc.ContentMarkAsReadResponse, error) {
	query := `
    UPDATE "Message"
    SET read = true
    WHERE "convId" = $1 AND "recipientId" = $2 AND read = false
  `

	_, err := ms.db.Exec(ctx, query, msg.Convid, msg.To)
	if err != nil {
		return &ipc.ContentMarkAsReadResponse{}, err
	}

	return &ipc.ContentMarkAsReadResponse{
		Convid: msg.Convid,
		Unread: 0,
		To:     msg.To,
		Read:   true,
	}, nil
}
