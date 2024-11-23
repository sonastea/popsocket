package popsocket

import (
	"context"
	"fmt"
	"time"

	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	"google.golang.org/protobuf/proto"
)

type MessageType int

const (
	EventMessageType MessageType = iota
	RegularMessageType
)

const ParseEventMessageError = "Failed to parse message as EventMessage or RegularMessage"

type ParsedMessage struct {
	Type         MessageType
	EventMessage *ipc.EventMessage
	Message      *ipc.Message
}

func (p *PopSocket) handleMessages(ctx context.Context, client client, recv []byte) {
	parsed, err := parseMessage(recv)
	if err != nil {
		p.LogError("[PARSE ERROR] " + err.Error() + " " + string(recv))
		return
	}

	switch parsed.Type {
	case EventMessageType:
		p.processEventMessage(ctx, client, parsed.EventMessage)
	case RegularMessageType:
		go p.processRegularMessage(recv, parsed)
	}
}

func parseMessage(recv []byte) (*ParsedMessage, error) {
	eventMsg := &ipc.EventMessage{}
	if err := proto.Unmarshal(recv, eventMsg); err == nil {
		if eventMsg.Event != ipc.EventType_UNKNOWN_TYPE {
			return &ParsedMessage{
				Type:         EventMessageType,
				EventMessage: eventMsg,
			}, nil
		}
	}

	regularMsg := &ipc.Message{}
	if err := proto.Unmarshal(recv, regularMsg); err == nil {
		return &ParsedMessage{
			Type:    RegularMessageType,
			Message: regularMsg,
		}, nil
	}

	return nil, fmt.Errorf(ParseEventMessageError)
}

// sanitizeCreatedAt ensures regular messages have a reliable and nonspoofed timestamp
func sanitizeCreatedAt(message *ipc.Message) {
	now := time.Now()
	createdAt, err := time.Parse(time.RFC3339, message.CreatedAt)
	if err != nil {
		Logger().Error(fmt.Sprintf("Failed to parse CreatedAt timestamp: %v", err))
		message.CreatedAt = now.Format(time.RFC3339)
		return
	}

	if createdAt.After(now) {
		message.CreatedAt = now.Format(time.RFC3339)
	}
}

func (p *PopSocket) processEventMessage(ctx context.Context, client client, m *ipc.EventMessage) {
	switch m.Event {
	case ipc.EventType_CONNECT:
		p.connect(client)
	case ipc.EventType_CONVERSATIONS:
		p.conversations(ctx, client)
	case ipc.EventType_MARK_AS_READ:
		p.read(ctx, client, m.GetReqRead())
	default:
		p.LogWarn("[UNHANDLED EventMessage] " + m.String())
	}
}

func (p *PopSocket) processRegularMessage(send []byte, m *ParsedMessage) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if recipients, ok := p.clients[m.Message.To]; ok {
		for _, client := range recipients {
			client.Send() <- send
		}
	}
	if sender, ok := p.clients[m.Message.From]; ok {
		for _, client := range sender {
			client.Send() <- send
		}
	}
}

func (p *PopSocket) writeEventMessage(client client, msg *ipc.EventMessage) error {
	if msg == nil {
		return fmt.Errorf("Marshal error: msg passed is nil")
	}

	encoded, _ := proto.Marshal(msg)
	client.Send() <- encoded

	return nil
}

func (p *PopSocket) connect(client client) {
	message := &ipc.EventMessage{
		Event: ipc.EventType_CONNECT,
		Content: &ipc.EventMessage_RespConnect{RespConnect: &ipc.ContentConnectResponse{
			Content: fmt.Sprintf(`{"id": %d}`, client.ID()),
		}},
	}

	if err := p.writeEventMessage(client, message); err != nil {
		p.LogError("connect", "WriteError", err.Error())
	}
}

func (p *PopSocket) conversations(ctx context.Context, client client) {
	result, err := p.messageStore.Convos(ctx, client.ID())
	if err != nil {
		p.LogError("Error getting client's conversations: %v", err)
	}

	message := &ipc.EventMessage{
		Event: ipc.EventType_CONVERSATIONS,
		Content: &ipc.EventMessage_RespConvos{
			RespConvos: &ipc.ContentConversationsResponse{
				Conversations: result.Conversations,
			},
		},
	}

	if err := p.writeEventMessage(client, message); err != nil {
		p.LogError("conversations", "WriteError", err.Error())
	}
}

func (p *PopSocket) read(ctx context.Context, client client, m *ipc.ContentMarkAsRead) {
	result, err := p.messageStore.UpdateAsRead(ctx, m)
	if err != nil {
		p.LogWarn(err.Error(), result)
	}

	message := &ipc.EventMessage{
		Event: ipc.EventType_MARK_AS_READ,
		Content: &ipc.EventMessage_RespRead{
			RespRead: result,
		},
	}

	if err := p.writeEventMessage(client, message); err != nil {
		p.LogError("read", "WriteError", err.Error())
	}
}
