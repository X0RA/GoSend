package storage

import (
	"testing"
)

func TestMessageCRUD(t *testing.T) {
	store := newTestStore(t)
	mustAddPeer(t, store, "self", "Self")
	mustAddPeer(t, store, "peer-1", "Peer One")

	oldSent := nowUnixMilli() - 10_000
	newSent := nowUnixMilli()
	received := nowUnixMilli()

	if err := store.SaveMessage(Message{
		MessageID:      "msg-old",
		FromDeviceID:   "self",
		ToDeviceID:     "peer-1",
		Content:        "old pending message",
		ContentType:    messageContentText,
		TimestampSent:  oldSent,
		DeliveryStatus: deliveryStatusPending,
		Signature:      "sig-old",
	}); err != nil {
		t.Fatalf("SaveMessage old pending failed: %v", err)
	}

	if err := store.SaveMessage(Message{
		MessageID:         "msg-new",
		FromDeviceID:      "self",
		ToDeviceID:        "peer-1",
		Content:           "new sent message",
		ContentType:       messageContentText,
		TimestampSent:     newSent,
		TimestampReceived: &received,
		IsRead:            true,
		DeliveryStatus:    deliveryStatusSent,
		Signature:         "sig-new",
	}); err != nil {
		t.Fatalf("SaveMessage new sent failed: %v", err)
	}

	if err := store.SaveMessage(Message{
		MessageID:      "msg-reply",
		FromDeviceID:   "peer-1",
		ToDeviceID:     "self",
		Content:        "reply",
		ContentType:    messageContentText,
		TimestampSent:  newSent + 1,
		DeliveryStatus: deliveryStatusDelivered,
		Signature:      "sig-reply",
	}); err != nil {
		t.Fatalf("SaveMessage reply failed: %v", err)
	}

	conversation, err := store.GetMessages("peer-1", 10, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(conversation) != 3 {
		t.Fatalf("expected 3 conversation messages, got %d", len(conversation))
	}
	if conversation[0].MessageID != "msg-old" || conversation[1].MessageID != "msg-new" {
		t.Fatalf("messages are not ordered by timestamp_sent ascending")
	}

	if err := store.MarkDelivered("msg-new"); err != nil {
		t.Fatalf("MarkDelivered failed: %v", err)
	}
	postMark, err := store.GetMessages("peer-1", 10, 0)
	if err != nil {
		t.Fatalf("GetMessages after MarkDelivered failed: %v", err)
	}
	var marked Message
	for _, msg := range postMark {
		if msg.MessageID == "msg-new" {
			marked = msg
			break
		}
	}
	if marked.DeliveryStatus != deliveryStatusDelivered {
		t.Fatalf("expected msg-new to be delivered, got %q", marked.DeliveryStatus)
	}

	pending, err := store.GetPendingMessages("peer-1")
	if err != nil {
		t.Fatalf("GetPendingMessages failed: %v", err)
	}
	if len(pending) != 1 || pending[0].MessageID != "msg-old" {
		t.Fatalf("expected only msg-old pending, got %+v", pending)
	}

	pruned, err := store.PruneExpiredQueue(nowUnixMilli() - 5_000)
	if err != nil {
		t.Fatalf("PruneExpiredQueue failed: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned row, got %d", pruned)
	}

	pendingAfterPrune, err := store.GetPendingMessages("peer-1")
	if err != nil {
		t.Fatalf("GetPendingMessages after prune failed: %v", err)
	}
	if len(pendingAfterPrune) != 0 {
		t.Fatalf("expected no pending messages after prune, got %d", len(pendingAfterPrune))
	}
}
