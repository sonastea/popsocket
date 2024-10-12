package db

import (
	"context"
	"testing"
)

type testcase struct {
	name       string
	connString string
	shouldFail bool
}

func TestNewPostgres(t *testing.T) {
	tests := []testcase{
		{
			name:       "Return a DB instance",
			connString: "postgresql://postgres:postgres@localhost:5432",
			shouldFail: false,
		},
		{
			name:       "Return an error",
			connString: "invalid-conn-string",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldFail {
				pg, err := NewPostgres(context.Background(), tt.connString)
				if pg != nil && err == nil {
					t.Fatalf("Expected new postgres to return an error")
				}
			} else {
				pg, err := NewPostgres(context.Background(), tt.connString)
				if pg == nil && err != nil {
					t.Fatalf("Expected a DB instance, got %s", err)
				}
			}
		})
	}
}
