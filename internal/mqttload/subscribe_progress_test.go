package mqttload

import "testing"

func TestSubscribeProgress_Snapshot(t *testing.T) {
	progress := NewSubscribeProgress()

	snapshot := progress.Snapshot()
	if snapshot.Received != 0 || !snapshot.LastMessageAt.IsZero() || !snapshot.SubscribedAt.IsZero() {
		t.Fatalf("expected a zero-value snapshot, got %+v", snapshot)
	}

	progress.recordMessage()
	progress.recordMessage()
	progress.recordSubscribed()

	snapshot = progress.Snapshot()
	if snapshot.Received != 2 {
		t.Errorf("expected Received 2, got %d", snapshot.Received)
	}
	if snapshot.LastMessageAt.IsZero() {
		t.Error("expected LastMessageAt to be set after recordMessage")
	}
	if snapshot.SubscribedAt.IsZero() {
		t.Error("expected SubscribedAt to be set after recordSubscribed")
	}
}

func TestSubscribeProgress_Done(t *testing.T) {
	progress := NewSubscribeProgress()

	select {
	case <-progress.Done():
		t.Fatal("expected Done to still be open")
	default:
	}

	progress.finish()

	select {
	case <-progress.Done():
	default:
		t.Fatal("expected Done to be closed")
	}
}
