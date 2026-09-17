package conversation

import (
	"fmt"
	"testing"
)

func TestStoreIsolatesConversations(t *testing.T) {
	store := NewStore()
	store.AppendExchange("a", "user a", "assistant a")
	store.AppendExchange("b", "user b", "assistant b")

	a := store.Context("a")
	b := store.Context("b")
	if len(a) != 2 || a[0].Content != "user a" || a[1].Content != "assistant a" {
		t.Fatalf("conversation a = %#v", a)
	}
	if len(b) != 2 || b[0].Content != "user b" || b[1].Content != "assistant b" {
		t.Fatalf("conversation b = %#v", b)
	}
}

func TestStoreBoundsMessageHistory(t *testing.T) {
	store := NewStore()
	for i := 0; i < maxMessages+5; i++ {
		store.AppendExchange("thread", fmt.Sprintf("user-%02d", i), "")
	}
	messages := store.Context("thread")
	if len(messages) != maxMessages {
		t.Fatalf("message count = %d; want %d", len(messages), maxMessages)
	}
	if messages[0].Content != "user-05" {
		t.Fatalf("oldest retained message = %q", messages[0].Content)
	}
}
