package popsocket

type messageType string

var MessageType = struct {
	Connect       messageType
	Conversations messageType
	MarkAsRead    messageType
	SetRecipient  messageType
}{
	Connect:       messageType("CONNECT"),
	Conversations: messageType("CONVERSATIONS"),
	MarkAsRead:    messageType("MARK_AS_READ"),
	SetRecipient:  messageType("SET_RECIPIENT"),
}

type Message struct {
	Event   messageType `json:"event"`
	Content string      `json:"content"`
}
