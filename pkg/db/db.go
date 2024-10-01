package db

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB interface {
	GetMessages(ctx context.Context, userID string) (*ConversationsResponse, error)
}

type postgres struct {
	db *pgxpool.Pool
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

type ConversationsResponse struct {
	Conversations []Conversation `json:"conversations"`
}

var conversations []struct {
	ID       string   `json:"id"`
	Messages []string `json:"messages"`
}

var (
	logger slog.Logger
	pg     *postgres
	pgOnce sync.Once
)

func NewPostgres(ctx context.Context, connString string) (*postgres, error) {
	pgOnce.Do(func() {
		db, err := pgxpool.New(ctx, connString)
		if err != nil {
			log.Fatal(fmt.Errorf("unable to create connection pool: %s", err))
		}
		pg = &postgres{db}
	})

	return pg, nil
}

func (pg *postgres) Ping(ctx context.Context) error {
	return pg.db.Ping(ctx)
}

func (pg *postgres) Close() {
	pg.db.Close()
}

func GetMessages(ctx context.Context, userID int) (*ConversationsResponse, error) {
	query := `
    WITH user_conversations AS (
      SELECT DISTINCT c.id, c.convid
      FROM kpoppop."Conversation" c
      JOIN kpoppop."_ConversationToUser" cu ON c.id = cu."A"
      WHERE cu."B" = $1
    ),
    conversation_users AS (
      SELECT uc.id, uc.convid, u.id as user_id, u.username, u.displayname, u.photo, u.status
      FROM user_conversations uc
      JOIN kpoppop."_ConversationToUser" cu ON uc.id = cu."A"
      JOIN kpoppop."User" u ON cu."B" = u.id
      WHERE u.id != $1
    ),
    conversation_messages AS (
      SELECT c.id, c.convid, m.*
      FROM user_conversations c
      LEFT JOIN kpoppop."Message" m ON c.convid = m."convId"
    )
    SELECT
      cm."recipientId" as id, cu.convid, cu.username, cu.displayname, cu.photo, cu.status,
      cm."recipientId", cm."userId", cm.content, cm."createdAt", cm."fromSelf", cm.read
    FROM conversation_users cu
    LEFT JOIN conversation_messages cm ON cu.convid = cm.convid
    ORDER BY cu.id, cm."createdAt" desc
    `

	rows, err := pg.db.Query(ctx, query, "1")
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
