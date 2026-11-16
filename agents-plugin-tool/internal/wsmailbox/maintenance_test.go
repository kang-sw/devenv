package wsmailbox

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswake"
)

func TestQueueChangesPublishOnlySuccessfulQueueMutations(t *testing.T) {
	path := filepath.Join(t.TempDir(), storeFileName)
	sub := wswake.Mailbox.SubscribeStores()
	defer sub.Close()
	if err := WithLock(path, func(store *StoreFile) error {
		store.Presence["owner"] = Presence{LastSeen: "heartbeat"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := sub.TakeStores(); len(got) != 0 {
		t.Fatalf("presence-only write published queues: %v", got)
	}
	message := Envelope{ReplyTo: "id:sender", Content: "message", SentAt: time.Now().Format(time.RFC3339)}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Queues = AppendQueue(store.Queues, "recipient", message)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := sub.TakeStores(); !reflect.DeepEqual(got, []string{path}) {
		t.Fatalf("queue change not published: %v", got)
	}
	failure := errors.New("abort")
	if err := WithLock(path, func(store *StoreFile) error {
		store.Queues["recipient"][0].Content = "not committed"
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := WithLock(path, func(*StoreFile) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := sub.TakeStores(); len(got) != 0 {
		t.Fatalf("failed/unchanged write published: %v", got)
	}
	reply := filepath.Join(t.TempDir(), replyRegistryName)
	if err := WithReplyLock(reply, func(store *ReplyStore) error {
		store.Entries["sender"] = ReplyEntry{LastSeen: "heartbeat"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(sub.TakeStores()) != 0 {
		t.Fatal("reply presence-only write published queues")
	}
	if err := PostSystem(reply, "sender", "", "report", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := sub.TakeStores(); !reflect.DeepEqual(got, []string{reply}) {
		t.Fatalf("reply queue not published: %v", got)
	}
}

func TestUndeliveredStoreDeadlineAuthorityAndMissingSources(t *testing.T) {
	paths := Paths{Config: wsconfig.Options{ConfigHome: t.TempDir()}, State: wsstate.Options{CacheHome: t.TempDir()}}
	sweep, err := NewUndeliveredSweep(paths)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := sweep.Stores()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, store := range stores {
		if _, _, err := sweep.Sweep(store, now, time.Minute); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(store.Path); !os.IsNotExist(err) {
			t.Fatalf("missing source was created: %s: %v", store.Path, err)
		}
	}
	foreign := filepath.Join(t.TempDir(), storeFileName)
	if _, ok := sweep.Target(foreign); ok {
		t.Fatal("foreign store admitted")
	}
	if _, _, err := sweep.Sweep(QueueStore{Path: foreign}, now, time.Minute); err == nil {
		t.Fatal("forged store admitted")
	}
	if err := WithReplyLock(sweep.reply, func(store *ReplyStore) error {
		store.Queues = AppendQueue(store.Queues, "exited", Envelope{ReplyTo: "id:sender", SentAt: now.Format(time.RFC3339), Content: "pending"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target, _ := sweep.Target(sweep.reply)
	posted, next, err := sweep.Sweep(target, now, time.Minute)
	if err != nil || posted != 0 || !next.Equal(now.Add(time.Minute)) {
		t.Fatalf("future deadline: posted=%d next=%v err=%v", posted, next, err)
	}
	// A replacement scheduler has no remembered receipt/deadline authority.
	restarted, _ := NewUndeliveredSweep(paths)
	posted, next, err = restarted.Sweep(target, next, time.Minute)
	if err != nil || posted != 1 || !next.IsZero() {
		t.Fatalf("deadline/restart: posted=%d next=%v err=%v", posted, next, err)
	}
	if posted, _, err := restarted.Sweep(target, now.Add(time.Hour), time.Minute); err != nil || posted != 0 {
		t.Fatalf("durable receipt replayed: %d %v", posted, err)
	}
	store, _ := LoadReplyStore(sweep.reply)
	if len(store.Queues["sender"]) != 1 || !store.Queues["exited"][0].UndeliveredNotified {
		t.Fatal("notice/receipt missing or original mail consumed")
	}
}

func TestUndeliveredFailedNoticeDoesNotDelayAnEarlierFutureDeadline(t *testing.T) {
	base := t.TempDir()
	blocked := filepath.Join(base, "blocked")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := Paths{Config: wsconfig.Options{ConfigHome: blocked}, State: wsstate.Options{CacheHome: t.TempDir()}}
	sweep, err := NewUndeliveredSweep(paths)
	if err != nil {
		t.Fatal(err)
	}
	target, ok := sweep.Target(filepath.Join(sweep.cache, "proj", "fixture", storeFileName))
	if !ok {
		t.Fatal("worktree target not admitted")
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := WithLock(target.Path, func(store *StoreFile) error {
		store.Queues["recipient"] = []Envelope{
			{From: "sender@machine", Content: "failed destination", SentAt: now.Add(-4 * time.Minute).Format(time.RFC3339)},
			{ReplyTo: "id:sender", Content: "next deadline", SentAt: now.Add(-3*time.Minute + 5*time.Second).Format(time.RFC3339)},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	posted, next, err := sweep.Sweep(target, now, 3*time.Minute)
	if err == nil || posted != 0 || !next.Equal(now.Add(5*time.Second)) {
		t.Fatalf("mixed retry/future: posted=%d next=%v err=%v", posted, next, err)
	}
	posted, _, err = sweep.Sweep(target, next, 3*time.Minute)
	if err == nil || posted != 1 {
		t.Fatalf("future notice delayed by other failure: %d %v", posted, err)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	posted, next, err = sweep.Sweep(target, now.Add(time.Minute), 3*time.Minute)
	if err != nil || posted != 1 || !next.IsZero() {
		t.Fatalf("failed destination recovery: %d %v %v", posted, next, err)
	}
}
