package popsocket

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	mock_client "github.com/sonastea/popsocket/internal/mock/client"
	"google.golang.org/protobuf/proto"
)

func TestParseMessage(t *testing.T) {
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
		m, err := parseMessage(send)
		if err != nil {
			t.Fatalf("Failed to parse event message: %s", err)
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
		m, err := parseMessage(send)
		if err != nil {
			t.Fatalf("Failed to parse event message: %s", err)
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

		m, err := parseMessage(invalidMsg)
		if err.Error() != ParseEventMessageError {
			t.Fatalf("Expected error message '%s', got %s", ParseEventMessageError, err.Error())
		}
		if m != nil {
			t.Fatalf("Expected ParsedMessage to be nil, got %v", m)
		}
	})
}

func TestHandleMessages(t *testing.T) {
	t.Run(("Expect PARSE ERROR early return"), func(t *testing.T) {
		logger, buf := newTestLogger()
		p := &PopSocket{
			logger: logger,
		}

		invalidMsg := []byte("invalid protobuf message")
		p.handleMessages(context.Background(), &Client{}, invalidMsg)
		if !strings.Contains(buf.String(), "[PARSE ERROR]") {
			t.Fatalf("Expected '[PARSE ERROR]' in logged message, got %s", buf.String())
		}
	})
}

func TestWriteEventMessage(t *testing.T) {
	p := &PopSocket{}
	c := mock_client.New("1", 1, "1")

	err := p.writeEventMessage(c, nil)
	if !strings.Contains(err.Error(), "msg passed is nil") {
		t.Fatalf("Expected marshal error upon passing nil msg, got %s", err.Error())
	}
}

func TestProcessRegularMessage(t *testing.T) {
	t.Run("Message between two users", func(t *testing.T) {
		logger, _ := newTestLogger()
		sender := mock_client.New("sender", 1, "1")
		receiver := mock_client.New("receiver", 9, "9")
		p := &PopSocket{
			logger: logger,
			clients: map[int32]map[string]client{
				1: {
					"sender": sender,
				},
				9: {
					"receiver": receiver,
				},
			},
			mu: sync.RWMutex{},
		}

		content := "foo-bar"
		m := &ipc.Message{
			Content: &content,
			To:      1,
			From:    9,
		}

		send, err := proto.Marshal(m)
		if err != nil {
			t.Fatalf("Failed to marshal ipc message")
		}

		p.processRegularMessage(send, m)

		select {
		case msg := <-receiver.Send():
			if !bytes.Equal(msg, send) {
				t.Fatal("[Receiver] Expected receiver client to get message")
			}
		}

		select {
		case msg := <-sender.Send():
			if !bytes.Equal(msg, send) {
				t.Fatal("[Sender] Expected sender client to get message")
			}
		}
	})

	t.Run("Message between self", func(t *testing.T) {
		logger, _ := newTestLogger()
		self := mock_client.New("self", 1, "1")
		p := &PopSocket{
			logger: logger,
			clients: map[int32]map[string]client{
				1: {
					"self": self,
				},
			},
			mu: sync.RWMutex{},
		}

		content := "self-message"
		m := &ipc.Message{
			Content:  &content,
			To:       1,
			From:     1,
			FromSelf: true,
		}

		send, err := proto.Marshal(m)
		if err != nil {
			t.Fatalf("Failed to marshal ipc message")
		}

		p.processRegularMessage(send, m)

		select {
		case msg := <-self.Send():
			if !bytes.Equal(msg, send) {
				t.Fatal("[Self] Expected self client to get message")
			}
		default:
			t.Fatal("[Self] Did not receive the expected message")
		}

		select {
		case <-self.Send():
			t.Fatal("[Self] Received duplicate message")
		default:
		}
	})
}
