package popsocket

import (
	"context"
	"strings"
	"testing"
	"time"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	"google.golang.org/protobuf/proto"
)

func TestParseEventMessage(t *testing.T) {
	t.Run("Parse EventMessage", func(t *testing.T) {
		message := &ipc.EventMessage{
			Event: ipc.EventType_MARK_AS_READ,
			Content: &ipc.EventMessage_ReqRead{
				ReqRead: &ipc.ContentMarkAsRead{
					Convid: "foo-bar",
					To:     1,
				},
			},
		}

		send, _ := proto.Marshal(message)
		m, err := parseEventMessage(send)
		if err != nil {
			t.Fatalf("Failed to parse event message: %s", err.Error())
		}

		if m.EventMessage.GetEvent() != ipc.EventType_MARK_AS_READ {
			t.Fatalf("Expected 'EventType_MARK_AS_READ', got %s", m.EventMessage.GetEvent())
		}
		if m.EventMessage.GetReqRead().GetConvid() != "foo-bar" {
			t.Fatalf("Expected convid to be 'foo-bar', got %s", m.EventMessage.GetReqRead().GetConvid())
		}
		if m.EventMessage.GetReqRead().GetTo() != 1 {
			t.Fatalf("Expected 'To' to be '1', got %d", m.EventMessage.GetReqRead().GetTo())
		}
	})

	t.Run("Parse RegularMessage", func(t *testing.T) {
		content := "lorem ipsum"
		message := &ipc.Message{
			Convid:    "foo-bar",
			To:        int32(9),
			From:      int32(324),
			Content:   &content,
			CreatedAt: time.Now().Format(time.RFC3339),
			FromSelf:  false,
			Read:      false,
		}

		send, _ := proto.Marshal(message)
		m, err := parseEventMessage(send)
		if err != nil {
			t.Fatalf("Failed to parse event message: %s", err.Error())
		}

		if m.Message.GetTo() != 9 {
			t.Fatalf("Expected to '1', got %d", m.Message.GetTo())
		}
		if m.Message.GetFrom() != 324 {
			t.Fatalf("Expected to '2', got %d", m.Message.GetFrom())
		}
		if m.Message.GetContent() != content {
			t.Fatalf("Expected message content '%s', got %s", content, m.Message.GetContent())
		}
	})

	t.Run("Expect failing to parse from invalid message", func(t *testing.T) {
		invalidMsg := []byte("invalid protobuf message")

		m, err := parseEventMessage(invalidMsg)
		if err.Error() != ParseEventMessageError {
			t.Fatalf("Expected error message '%s', got %s", ParseEventMessageError, err.Error())
		}
		if m != nil {
			t.Fatalf("Expected ParsedMessage to be nil, got %v", m)
		}
	})
}

func TestProcessMessage(t *testing.T) {
	t.Run(("Expect PARSE ERROR early return"), func(t *testing.T) {
		logger, buf := newTestLogger()
		p := &PopSocket{
			logger: logger,
		}

		invalidMsg := []byte("invalid protobuf message")
		p.processMessages(context.Background(), &Client{}, invalidMsg)
		if !strings.Contains(buf.String(), "[PARSE ERROR]") {
			t.Fatalf("Expected '[PARSE ERROR]' in logged message, got %s", buf.String())
		}
	})
}
