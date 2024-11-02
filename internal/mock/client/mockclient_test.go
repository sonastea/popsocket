package mock_client

import "testing"

func TestConn(t *testing.T) {
	c := New("1", 1, "1")

	if c.Conn() != nil {
		t.Fatalf("Expected client's conn to be nil, got %v", c.Conn())
	}
}

func TestConnID(t *testing.T) {
	c := New("1", 1, "1")

	if c.ConnID() != "1" {
		t.Fatalf("Expected client's connID to be '1', got %s", c.ConnID())
	}
}

func TestID(t *testing.T) {
	c := New("1", 1, "1")

	if c.ID() != 1 {
		t.Fatalf("Expected client's ID to be '1', got %d", c.ID())
	}
}

func TestSend(t *testing.T) {
	c := New("1", 1, "1")

	if len(c.Send()) != 0 {
		t.Fatalf("Expected client's send channel to be of length 0, got %d", len(c.Send()))
	}
}
