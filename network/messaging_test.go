package network

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	appcrypto "gosend/crypto"
	"gosend/storage"
)

func TestMessageDeliveryUpdatesStatusToDelivered(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	messageID, err := a.manager.SendTextMessage("peer-b", "hello from A")
	if err != nil {
		t.Fatalf("SendTextMessage failed: %v", err)
	}

	waitForMessageStatus(t, a.store, messageID, deliveryStatusDelivered, 4*time.Second)

	received := waitForMessage(t, b.store, messageID, 4*time.Second)
	if received.Content != "hello from A" {
		t.Fatalf("unexpected received content: %q", received.Content)
	}
	if received.FromDeviceID != "peer-a" || received.ToDeviceID != "peer-b" {
		t.Fatalf("unexpected sender/recipient on received message: %+v", received)
	}
}

func TestOfflineQueueDrainsAfterReconnect(t *testing.T) {
	bPort := freeTCPPort(t)

	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	bIdentity := generateIdentity(t, "peer-b", "Peer B")
	bStoreDir := t.TempDir()
	bStore, _, err := storage.Open(bStoreDir)
	if err != nil {
		t.Fatalf("open B store: %v", err)
	}
	defer func() {
		_ = bStore.Close()
	}()

	b := newTestManagerWithParts(t, bStore, bIdentity, "127.0.0.1:"+bPort)
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	b.stop()
	waitForPeerStatus(t, a.store, "peer-b", peerStatusOffline, 5*time.Second)

	messageID, err := a.manager.SendTextMessage("peer-b", "queued while offline")
	if err != nil {
		t.Fatalf("SendTextMessage while offline failed: %v", err)
	}
	waitForMessageStatus(t, a.store, messageID, deliveryStatusPending, 2*time.Second)

	b2 := newTestManagerWithParts(t, bStore, bIdentity, "127.0.0.1:"+bPort)
	defer b2.stop()

	waitForMessageStatus(t, a.store, messageID, deliveryStatusDelivered, 8*time.Second)
	waitForMessage(t, b2.store, messageID, 8*time.Second)
}

func TestReplayDuplicateMessageIDRejected(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	messageID := uuid.NewString()
	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	seq1 := conn.NextSendSequence()
	if err := sendManualEncryptedMessage(a.manager, "peer-b", messageID, seq1, "first", false); err != nil {
		t.Fatalf("send first message failed: %v", err)
	}
	seq2 := conn.NextSendSequence()
	if err := sendManualEncryptedMessage(a.manager, "peer-b", messageID, seq2, "replay", false); err != nil {
		t.Fatalf("send replay message failed: %v", err)
	}

	waitForMessage(t, b.store, messageID, 3*time.Second)
	time.Sleep(200 * time.Millisecond)

	conversation, err := b.store.GetMessages("peer-a", 10, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(conversation) != 1 {
		t.Fatalf("expected exactly 1 stored message after replay attempt, got %d", len(conversation))
	}
}

func TestOutOfSequenceMessageRejected(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	firstID := uuid.NewString()
	seq := conn.NextSendSequence()
	if err := sendManualEncryptedMessage(a.manager, "peer-b", firstID, seq, "valid sequence", false); err != nil {
		t.Fatalf("send first message failed: %v", err)
	}

	secondID := uuid.NewString()
	if err := sendManualEncryptedMessage(a.manager, "peer-b", secondID, seq, "replayed sequence", false); err != nil {
		t.Fatalf("send out-of-sequence message failed: %v", err)
	}

	waitForMessage(t, b.store, firstID, 3*time.Second)
	ensureMessageAbsent(t, b.store, secondID, 600*time.Millisecond)
}

func TestTamperedSignatureRejectedWithErrorResponse(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	messageID := uuid.NewString()
	seq := conn.NextSendSequence()
	if err := sendManualEncryptedMessage(a.manager, "peer-b", messageID, seq, "tampered", true); err != nil {
		t.Fatalf("send tampered message failed: %v", err)
	}

	ensureMessageAbsent(t, b.store, messageID, 600*time.Millisecond)
	waitForManagerErrorContaining(t, a.manager.Errors(), "invalid_signature", 3*time.Second)
}

func TestTamperedMessageMetadataRejectedWithErrorResponse(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	originalID := uuid.NewString()
	tamperedID := uuid.NewString()
	seq := conn.NextSendSequence()
	if err := sendManualEncryptedMessageWithTamperedMessageID(a.manager, "peer-b", originalID, tamperedID, seq, "tampered metadata"); err != nil {
		t.Fatalf("send tampered metadata message failed: %v", err)
	}

	ensureMessageAbsent(t, b.store, tamperedID, 600*time.Millisecond)
	waitForManagerErrorContaining(t, a.manager.Errors(), "invalid_signature", 3*time.Second)
}

func TestAckFromWrongConnectionPeerDoesNotMarkDelivered(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	c := newTestManager(t, testManagerConfig{
		deviceID: "peer-c",
		name:     "Peer C",
	})
	defer c.stop()

	if _, err := a.manager.Connect(c.addr()); err != nil {
		t.Fatalf("A connect C failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-c", c.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	victim := generateIdentity(t, "peer-b", "Peer B")
	now := time.Now().UnixMilli()
	if err := a.store.AddPeer(storage.Peer{
		DeviceID:          "peer-b",
		DeviceName:        "Peer B",
		Ed25519PublicKey:  base64.StdEncoding.EncodeToString(victim.Ed25519PublicKey),
		KeyFingerprint:    appcrypto.KeyFingerprint(victim.Ed25519PublicKey),
		Status:            peerStatusOffline,
		AddedTimestamp:    now,
		LastSeenTimestamp: &now,
	}); err != nil {
		t.Fatalf("seed peer-b failed: %v", err)
	}

	messageID := uuid.NewString()
	if err := a.store.SaveMessage(storage.Message{
		MessageID:      messageID,
		FromDeviceID:   "peer-a",
		ToDeviceID:     "peer-b",
		Content:        "spoof target",
		ContentType:    messageContentTypeText,
		TimestampSent:  time.Now().UnixMilli(),
		DeliveryStatus: deliveryStatusSent,
	}); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	conn := c.manager.getConnection("peer-a")
	if conn == nil {
		t.Fatalf("expected connection from C to A")
	}
	if err := conn.SendMessage(AckMessage{
		Type:         TypeAck,
		MessageID:    messageID,
		FromDeviceID: "peer-b",
		Status:       deliveryStatusDelivered,
		Timestamp:    time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("send spoofed ack failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	msg, err := a.store.GetMessageByID(messageID)
	if err != nil {
		t.Fatalf("GetMessageByID failed: %v", err)
	}
	if msg.DeliveryStatus != deliveryStatusSent {
		t.Fatalf("expected delivery status to remain %q, got %q", deliveryStatusSent, msg.DeliveryStatus)
	}
}

func sendManualEncryptedMessage(sender *PeerManager, peerDeviceID, messageID string, sequence uint64, content string, tamperSignature bool) error {
	conn := sender.getConnection(peerDeviceID)
	if conn == nil {
		return errors.New("no active connection")
	}

	ciphertext, iv, err := appcrypto.Encrypt(conn.SessionKey(), []byte(content))
	if err != nil {
		return err
	}
	message := EncryptedMessage{
		Type:             TypeMessage,
		MessageID:        messageID,
		FromDeviceID:     sender.options.Identity.DeviceID,
		ToDeviceID:       peerDeviceID,
		ContentType:      messageContentTypeText,
		Sequence:         sequence,
		EncryptedContent: base64.StdEncoding.EncodeToString(ciphertext),
		IV:               base64.StdEncoding.EncodeToString(iv),
		Timestamp:        time.Now().UnixMilli(),
	}

	signature, err := signManualEncryptedMessage(sender, message)
	if err != nil {
		return err
	}
	if tamperSignature && len(signature) > 0 {
		signature[0] ^= 0xFF
	}
	message.Signature = base64.StdEncoding.EncodeToString(signature)

	return conn.SendMessage(message)
}

func sendManualEncryptedMessageWithTamperedMessageID(sender *PeerManager, peerDeviceID, originalMessageID, tamperedMessageID string, sequence uint64, content string) error {
	conn := sender.getConnection(peerDeviceID)
	if conn == nil {
		return errors.New("no active connection")
	}

	ciphertext, iv, err := appcrypto.Encrypt(conn.SessionKey(), []byte(content))
	if err != nil {
		return err
	}
	message := EncryptedMessage{
		Type:             TypeMessage,
		MessageID:        originalMessageID,
		FromDeviceID:     sender.options.Identity.DeviceID,
		ToDeviceID:       peerDeviceID,
		ContentType:      messageContentTypeText,
		Sequence:         sequence,
		EncryptedContent: base64.StdEncoding.EncodeToString(ciphertext),
		IV:               base64.StdEncoding.EncodeToString(iv),
		Timestamp:        time.Now().UnixMilli(),
	}
	signature, err := signManualEncryptedMessage(sender, message)
	if err != nil {
		return err
	}

	message.MessageID = tamperedMessageID
	message.Signature = base64.StdEncoding.EncodeToString(signature)
	return conn.SendMessage(message)
}

func signManualEncryptedMessage(sender *PeerManager, message EncryptedMessage) ([]byte, error) {
	signable := message
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return nil, err
	}
	return appcrypto.Sign(sender.options.Identity.Ed25519PrivateKey, raw)
}

func waitForMessageStatus(t *testing.T, store *storage.Store, messageID, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, err := store.GetMessageByID(messageID)
		if err == nil && msg.DeliveryStatus == expected {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	msg, err := store.GetMessageByID(messageID)
	if err != nil {
		t.Fatalf("timed out waiting for message %q status=%q, final error=%v", messageID, expected, err)
	}
	t.Fatalf("timed out waiting for message %q status=%q, final=%q", messageID, expected, msg.DeliveryStatus)
}

func waitForMessage(t *testing.T, store *storage.Store, messageID string, timeout time.Duration) *storage.Message {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, err := store.GetMessageByID(messageID)
		if err == nil {
			return msg
		}
		time.Sleep(25 * time.Millisecond)
	}

	msg, err := store.GetMessageByID(messageID)
	t.Fatalf("timed out waiting for message %q, final message=%+v final err=%v", messageID, msg, err)
	return nil
}

func ensureMessageAbsent(t *testing.T, store *storage.Store, messageID string, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		msg, err := store.GetMessageByID(messageID)
		if err == nil && msg != nil {
			t.Fatalf("expected message %q to stay absent, but it was stored", messageID)
		}
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("unexpected error while checking message absence: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForManagerErrorContaining(t *testing.T, errs <-chan error, needle string, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case err := <-errs:
			if err != nil && strings.Contains(err.Error(), needle) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for manager error containing %q", needle)
		}
	}
}
