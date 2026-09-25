package mqttload

import "testing"

func TestPublishProgress_SnapshotAggregatesClientCounters(t *testing.T) {
	progress := NewPublishProgress(2)

	progress.counterFor(0).published.Add(3)
	progress.counterFor(0).acked.Add(2)
	progress.counterFor(1).published.Add(1)
	progress.counterFor(1).failed.Add(1)

	snapshot := progress.Snapshot()
	if snapshot.Published != 4 {
		t.Errorf("Published = %d, want 4", snapshot.Published)
	}
	if snapshot.Acked != 2 {
		t.Errorf("Acked = %d, want 2", snapshot.Acked)
	}
	if snapshot.Failed != 1 {
		t.Errorf("Failed = %d, want 1", snapshot.Failed)
	}
	if snapshot.ClientsTotal != 2 {
		t.Errorf("ClientsTotal = %d, want 2", snapshot.ClientsTotal)
	}
}

func TestPublishProgress_ConnectAndFinishLifecycle(t *testing.T) {
	progress := NewPublishProgress(1)

	select {
	case <-progress.ConnectFinished():
		t.Fatal("expected ConnectFinished to still be open")
	default:
	}

	progress.clientConnected()
	if got := progress.Snapshot().ClientsConnected; got != 1 {
		t.Errorf("ClientsConnected = %d, want 1", got)
	}

	progress.finishConnecting(true)

	select {
	case <-progress.ConnectFinished():
	default:
		t.Fatal("expected ConnectFinished to be closed")
	}
	if progress.Snapshot().PublishStartedAt.IsZero() {
		t.Error("expected PublishStartedAt to be set")
	}

	progress.finish()

	select {
	case <-progress.Done():
	default:
		t.Fatal("expected Done to be closed")
	}
	if progress.Snapshot().FinishedAt.IsZero() {
		t.Error("expected FinishedAt to be set")
	}
}

func TestPublishProgress_FinishConnectingFalseLeavesPublishStartedAtZero(t *testing.T) {
	progress := NewPublishProgress(1)
	progress.finishConnecting(false)

	if !progress.Snapshot().PublishStartedAt.IsZero() {
		t.Error("expected PublishStartedAt to stay zero when connecting did not start publishing")
	}

	select {
	case <-progress.ConnectFinished():
	default:
		t.Fatal("expected ConnectFinished to be closed")
	}
}
