package popsocket

import (
	"testing"

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
}
