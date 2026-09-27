// Файл: internal/telegram/domain/telegram_test.go
package domain

import (
	"errors"
	"testing"
)

func TestPeerKeyRoundTrip(t *testing.T) {
	p := Peer{Type: PeerChannel, ID: 1234567890, AccessHash: -987654321}
	got, err := ParsePeerKey(p.Key())
	if err != nil || got != p {
		t.Fatalf("got %+v err %v", got, err)
	}
	for _, bad := range []string{"", "channel:1", "user:1:2", "chat:x:0", "chat:1:y"} {
		if _, err := ParsePeerKey(bad); !errors.Is(err, ErrBadPeerKey) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}
