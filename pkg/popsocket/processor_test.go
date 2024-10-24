package popsocket

import (
	"testing"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	"google.golang.org/protobuf/proto"
)

func TestParseEventMessage(t *testing.T) {
	message := ipc.EventMessage{
		Event: ipc.EventType_MARK_AS_READ,
		Content: &ipc.EventMessage_ReqRead{
			ReqRead: &ipc.ContentMarkAsRead{
				Convid: "foo-bar",
				To:     1,
			},
		},
	}

	out, err := proto.Marshal(&message)
	if err != nil {
		t.Fatalf("Failed to encode event message: %v", err)
	}

	msg, err := parseEventMessage(out)
	if err != nil {
		t.Fatalf("Failed to parse event message: %v", err)
	}

	if msg.GetReqRead().GetConvid() != "foo-bar" {
		t.Fatalf("Expected convid to be 'foo-bar', got %s", msg.GetReqRead().GetConvid())
	}

	if msg.GetReqRead().GetTo() != 1 {
		t.Fatalf("Expected 'To' to be '1', got %d", msg.GetReqRead().GetTo())
	}
}
