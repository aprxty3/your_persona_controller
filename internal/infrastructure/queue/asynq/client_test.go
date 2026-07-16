package asynq

import "testing"

func TestNewAsynqClient_EmptyAddr_ReturnsError(t *testing.T) {
	client, err := NewAsynqClient("", "", 0)
	if err == nil {
		t.Fatal("expected an error when redis address is empty")
	}
	if client != nil {
		t.Fatal("expected a nil client on error")
	}
}

// NewAsynqClient only constructs the client (asynq lazily dials Redis on
// first Enqueue) — no network connection is expected here.
func TestNewAsynqClient_ValidAddr_ReturnsClient(t *testing.T) {
	client, err := NewAsynqClient("localhost:6379", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected a non-nil client")
	}
	defer client.Close()
}
