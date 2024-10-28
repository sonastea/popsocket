package popsocket

import (
	"context"
	"fmt"
	"time"

	"github.com/coder/websocket"
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

func parseEventMessage(recv []byte) (*ParsedMessage, error) {
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
		fmt.Println(len(recv))
		// TODO: Generate CONVID(UUID) and CreatedAt in database layer?
		regularMsg.CreatedAt = time.Now().Format(time.RFC3339)
		return &ParsedMessage{
			Type:    RegularMessageType,
			Message: regularMsg,
		}, nil
	}

	return nil, fmt.Errorf(ParseEventMessageError)
}

func (p *PopSocket) processMessages(ctx context.Context, client client, recv []byte) {
	parsed, err := parseEventMessage(recv)
	if err != nil {
		p.LogError("[PARSE ERROR] " + err.Error() + " " + string(recv))
		return
	}

	switch parsed.Type {
	case EventMessageType:
		p.processEventMessage(ctx, client, parsed.EventMessage)
	case RegularMessageType:
		p.processRegularMessage(ctx, parsed.Message)
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

func (p *PopSocket) processRegularMessage(ctx context.Context, m *ipc.Message) {
	encoded, err := proto.Marshal(m)
	if err != nil {
		p.LogError("Marshal error: %w", err)
	}

	for _, recipient := range p.clients[m.To] {
		if err := recipient.Conn().Write(ctx, websocket.MessageBinary, encoded); err != nil {
			p.LogError("Websocket write error: %w", err)
		}
	}

	for _, recipient := range p.clients[m.From] {
		if err := recipient.Conn().Write(ctx, websocket.MessageBinary, encoded); err != nil {
			p.LogError("Websocket write error: %w", err)
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

	if err := p.writeMessage(ctx, client, message); err != nil {
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

	if err := p.writeMessage(ctx, client, message); err != nil {
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

	if err := p.writeMessage(ctx, client, message); err != nil {
		p.LogError("read", "WriteError", err.Error())
	}
}

func (p *PopSocket) writeMessage(ctx context.Context, client client, msg interface{}) error {
	encoded, err := proto.Marshal(msg.(proto.Message))
	if err != nil {
		return fmt.Errorf("Marshal error: %w", err)
	}

	if err := client.Conn().Write(ctx, websocket.MessageBinary, encoded); err != nil {
		return fmt.Errorf("Websocket write error: %w", err)
	}

	return nil
}
