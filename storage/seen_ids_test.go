package storage

import (
	"testing"
)

func TestSeenMessageIDsOperations(t *testing.T) {
	store := newTestStore(t)

	oldTimestamp := nowUnixMilli() - 10_000
	newTimestamp := nowUnixMilli()

	if err := store.InsertSeenID("msg-old", oldTimestamp); err != nil {
		t.Fatalf("InsertSeenID old failed: %v", err)
	}
	if err := store.InsertSeenID("msg-new", newTimestamp); err != nil {
		t.Fatalf("InsertSeenID new failed: %v", err)
	}

	seen, err := store.HasSeenID("msg-old")
	if err != nil {
		t.Fatalf("HasSeenID old failed: %v", err)
	}
	if !seen {
		t.Fatalf("expected msg-old to exist in seen_message_ids")
	}

	seen, err = store.HasSeenID("missing")
	if err != nil {
		t.Fatalf("HasSeenID missing failed: %v", err)
	}
	if seen {
		t.Fatalf("expected missing message ID to be unseen")
	}

	pruned, err := store.PruneOldEntries(nowUnixMilli() - 5_000)
	if err != nil {
		t.Fatalf("PruneOldEntries failed: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned seen message ID, got %d", pruned)
	}

	seenOld, err := store.HasSeenID("msg-old")
	if err != nil {
		t.Fatalf("HasSeenID msg-old after prune failed: %v", err)
	}
	seenNew, err := store.HasSeenID("msg-new")
	if err != nil {
		t.Fatalf("HasSeenID msg-new after prune failed: %v", err)
	}
	if seenOld {
		t.Fatalf("expected msg-old to be pruned")
	}
	if !seenNew {
		t.Fatalf("expected msg-new to remain after prune")
	}
}
