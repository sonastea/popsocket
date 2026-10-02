package popsocket

import (
	"strings"
	"testing"
	"time"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	"google.golang.org/protobuf/proto"
)

// benchEventMessage returns a marshalled EventMessage as a client would send it.
func benchEventMessage(b *testing.B) []byte {
	b.Helper()
	message := &ipc.EventMessage{
		Event: ipc.EventType_MARK_AS_READ,
		Content: &ipc.EventMessage_ReqRead{
			ReqRead: &ipc.ContentMarkAsRead{
				Convid: "foo-bar",
				To:     1,
				From:   2,
			},
		},
	}
	send, err := proto.Marshal(message)
	if err != nil {
		b.Fatalf("Failed to marshal event message: %s", err)
	}
	return send
}

// benchRegularMessage returns a marshalled Message as a client would send it.
func benchRegularMessage(b *testing.B) []byte {
	b.Helper()
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
	send, err := proto.Marshal(message)
	if err != nil {
		b.Fatalf("Failed to marshal regular message: %s", err)
	}
	return send
}

// recycleParsed returns a parsed event message to the pool the same way
// handleMessages does once processing completes.
func recycleParsed(parsed *ParsedMessage) {
	if parsed != nil && parsed.Type == EventMessageType {
		eventMessagePool.Put(parsed.EventMessage)
	}
}

func BenchmarkParseMessageEvent(b *testing.B) {
	send := benchEventMessage(b)
	b.ReportAllocs()
	for b.Loop() {
		parsed, err := parseMessage(send)
		if err != nil {
			b.Fatalf("Failed to parse event message: %s", err)
		}
		recycleParsed(parsed)
	}
}

func BenchmarkParseMessageRegular(b *testing.B) {
	send := benchRegularMessage(b)
	b.ReportAllocs()
	for b.Loop() {
		parsed, err := parseMessage(send)
		if err != nil {
			b.Fatalf("Failed to parse regular message: %s", err)
		}
		recycleParsed(parsed)
	}
}

func BenchmarkParseMessageRegularLarge(b *testing.B) {
	content := strings.Repeat("lorem ipsum ", 75)
	send, err := proto.Marshal(&ipc.Message{
		Convid:    "foo-bar",
		To:        9,
		From:      324,
		Content:   &content,
		CreatedAt: "2026-09-30T12:00:00Z",
	})
	if err != nil {
		b.Fatalf("Failed to marshal regular message: %s", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		parsed, err := parseMessage(send)
		if err != nil {
			b.Fatalf("Failed to parse regular message: %s", err)
		}
		recycleParsed(parsed)
	}
}
