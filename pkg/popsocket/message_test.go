package popsocket

import "testing"

func TestFuncType(t *testing.T) {
	type testcase struct {
		nameAndType  string
		contractType MessageInterface
	}

	tests := []testcase{
		{
			nameAndType:  EventMessageType.Connect.Type(),
			contractType: &EventMessage{Event: EventMessageType.Connect},
		},
		{
			nameAndType:  "Message",
			contractType: &Message{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.nameAndType, func(t *testing.T) {
			if tt.contractType.Type() != tt.nameAndType {
				t.Errorf("Expected %v interface to return of type %v, got %v", tt.contractType.Type(), tt.nameAndType, tt.contractType)
			}
		})
	}
}
