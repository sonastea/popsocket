package popsocket

import (
	"testing"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	"google.golang.org/protobuf/proto"
)

func FuzzParseMessage(f *testing.F) {
	for _, message := range []proto.Message{
		&ipc.EventMessage{
			Event: ipc.EventType_MARK_AS_READ,
			Content: &ipc.EventMessage_ReqRead{
				ReqRead: &ipc.ContentMarkAsRead{Convid: "foo-bar", To: 1, From: 2},
			},
		},
		&ipc.Message{
			Convid:    "foo-bar",
			To:        9,
			From:      324,
			Content:   proto.String("lorem ipsum"),
			CreatedAt: "2026-09-30T12:00:00Z",
		},
		&ipc.Message{To: 9, From: 324, Content: proto.String("new conversation")},
	} {
		data, err := proto.Marshal(message)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	for _, data := range [][]byte{
		{},
		[]byte("invalid protobuf message"),
		{0x08, 1},                               // CONNECT
		{0x08, 0},                               // UNKNOWN_TYPE falls back to Message.
		{0x08, 99},                              // Unknown nonzero event types remain events.
		{0x08, 1, 0x08, 0},                      // Last duplicate event field wins.
		{0x08, 0, 0x08, 1},                      // A later nonzero event wins.
		{0x10, 9, 0x08, 1},                      // Event field after another field.
		{0x0a, 1, 'x', 0x08, 1},                 // Both field-1 wire types: event takes precedence.
		{0x22, 1, 'x', 0x08, 1},                 // Invalid nested event content is a valid Message.
		{0x22, 0, 0x08, 1},                      // Content before the event discriminator.
		{0x08, 1, 0x22, 1, 0xff},                // Invalid nested content and invalid UTF-8.
		{0x88, 0, 1},                            // Non-minimal event tag encoding.
		{0x08, 0x81, 0},                         // Non-minimal enum encoding.
		{0x53, 0x08, 1, 0x54},                   // An event field inside an unknown group is not top-level.
		{0x53, 0x08, 1, 0x54, 0x08, 1},          // Unknown group before a top-level event.
		{0x53, 0x08, 1, 0x5c, 0x08, 1},          // Mismatched group terminator.
		{0x55, 0, 0, 0, 0, 0x08, 1},             // Unknown fixed32 field before an event.
		{0x51, 0, 0, 0, 0, 0, 0, 0, 0, 0x08, 1}, // Unknown fixed64 field before an event.
		{0x08},                                  // Truncated varint.
		{0x0a, 2, 'x'},                          // Truncated string.
	} {
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Use the protobuf decoders as an oracle for the event-first protocol,
		// including unknown fields, malformed payloads, and regular-message fallback.
		want := &ParsedMessage{}
		event := new(ipc.EventMessage)
		if err := proto.Unmarshal(data, event); err == nil && event.Event != ipc.EventType_UNKNOWN_TYPE {
			want.Type = EventMessageType
			want.EventMessage = event
		} else {
			message := new(ipc.Message)
			if err := proto.Unmarshal(data, message); err != nil {
				want = nil
			} else {
				want.Type = RegularMessageType
				want.Message = message
			}
		}

		got, err := parseMessage(data)
		defer recycleParsed(got)
		if want == nil {
			if got != nil || err == nil || err.Error() != ParseEventMessageError {
				t.Fatalf("Expected parse error, got message %v, error %v", got, err)
			}
			return
		}
		if err != nil || got == nil {
			t.Fatalf("Expected message type %v, got message %v, error %v", want.Type, got, err)
		}
		if got.Type != want.Type || !proto.Equal(got.EventMessage, want.EventMessage) || !proto.Equal(got.Message, want.Message) {
			t.Fatalf("Parse result differs from protobuf decoders: got %+v, want %+v", got, want)
		}
	})
}
