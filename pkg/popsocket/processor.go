package popsocket

import (
	"context"
	"fmt"

	"github.com/coder/websocket"
	ipc "github.com/sonastea/kpoppop-grpc/ipc/go"
	"google.golang.org/protobuf/proto"
)

func parseEventMessage(recv []byte) (*ipc.EventMessage, error) {
	var message ipc.EventMessage

	err := proto.Unmarshal(recv, &message)
	if err != nil {
		return nil, err
	}

	return &message, nil
}

func (p *PopSocket) processMessages(ctx context.Context, client client, recv []byte) {
	message, err := parseEventMessage(recv)
	if err != nil {
		p.LogError("[UNHANDLED recv] " + message.String() + err.Error())
		return
	}

	switch message.Event {
	case ipc.EventType_CONNECT:
		p.connect(ctx, client)
	case ipc.EventType_CONVERSATIONS:
		p.conversations(ctx, client)
	case ipc.EventType_MARK_AS_READ:
		p.read(ctx, client, message.GetReqRead())
	default:
		p.LogWarn("[UNHANDLED recv] " + message.String())
	}
}

func (p *PopSocket) connect(ctx context.Context, client client) {
	m := &ipc.EventMessage{
		Event: ipc.EventType_CONNECT,
		Content: &ipc.EventMessage_RespConnect{RespConnect: &ipc.ContentConnectResponse{
			Content: fmt.Sprintf(`{"id": %d}`, client.ID()),
		}},
	}

	encoded, err := proto.Marshal(m)
	if err != nil {
		p.LogError("ConnectEvent", "MarshalError", err.Error())
	}

	err = client.Conn().Write(ctx, websocket.MessageBinary, encoded)
}

func (p *PopSocket) conversations(ctx context.Context, client client) {
	result, err := p.messageStore.Convos(ctx, client.ID())
	if err != nil {
		p.LogError("Error getting client's conversations: %v", err)
	}

	m := &ipc.EventMessage{
		Event: ipc.EventType_CONVERSATIONS,
		Content: &ipc.EventMessage_RespConvos{
			RespConvos: &ipc.ContentConversationsResponse{
				Conversations: result.Conversations,
			},
		},
	}

	encoded, err := proto.Marshal(m)
	if err != nil {
		Logger().Error("Marshal", "error", err.Error())
	}

	err = client.Conn().Write(ctx, websocket.MessageBinary, encoded)
}

func (p *PopSocket) read(ctx context.Context, client client, msg *ipc.ContentMarkAsRead) {
	result, err := p.messageStore.UpdateAsRead(ctx, msg)
	if err != nil {
		p.LogWarn(err.Error(), result)
	}

	m := &ipc.EventMessage{
		Event: ipc.EventType_MARK_AS_READ,
		Content: &ipc.EventMessage_RespRead{
			RespRead: result,
		},
	}

	encoded, err := proto.Marshal(m)
	if err != nil {
		Logger().Error("Marshal", "error", err.Error())
	}

	err = client.Conn().Write(ctx, websocket.MessageBinary, encoded)
}
