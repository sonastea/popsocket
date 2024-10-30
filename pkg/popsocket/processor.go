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
		// TODO: Generate CONVID(UUID) and CreatedAt in database layer?
		regularMsg.CreatedAt = time.Now().Format(time.RFC3339)
		return &ParsedMessage{
			Type:    RegularMessageType,
			Message: regularMsg,
		}, nil
	}

	return nil, fmt.Errorf(ParseEventMessageError)
}

func (p *PopSocket) handleMessages(ctx context.Context, client client, recv []byte) {
	parsed, err := parseMessage(recv)
	if err != nil {
		p.LogError("[PARSE ERROR] " + err.Error() + " " + string(recv))
		return
	}

	switch parsed.Type {
	case EventMessageType:
		go p.processEventMessage(ctx, client, parsed.EventMessage)
	case RegularMessageType:
		go p.processRegularMessage(recv, parsed)
	}
}

func (p *PopSocket) processEventMessage(ctx context.Context, client client, m *ipc.EventMessage) {
	switch m.Event {
	case ipc.EventType_CONNECT:
		p.connect(ctx, client)
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

func (p *PopSocket) connect(ctx context.Context, client client) {
	message := &ipc.EventMessage{
		Event: ipc.EventType_CONNECT,
		Content: &ipc.EventMessage_RespConnect{RespConnect: &ipc.ContentConnectResponse{
			Content: fmt.Sprintf(`{"id": %d}`, client.ID()),
		}},
	}

	if err := p.writeEventMessage(ctx, client, message); err != nil {
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

	if err := p.writeEventMessage(ctx, client, message); err != nil {
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

	if err := p.writeEventMessage(ctx, client, message); err != nil {
		p.LogError("read", "WriteError", err.Error())
	}
}

func (p *PopSocket) writeEventMessage(ctx context.Context, client client, msg interface{}) error {
	encoded, err := proto.Marshal(msg.(proto.Message))
	if err != nil {
		return fmt.Errorf("Marshal error: %w", err)
	}

	client.Send() <- encoded

	return nil
}
