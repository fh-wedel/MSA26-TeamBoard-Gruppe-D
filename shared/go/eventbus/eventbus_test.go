package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ─── Pure unit tests (no broker required) ────────────────────────────────────

func TestEnvelope_JSONRoundTrip(t *testing.T) {
	in := Envelope{
		EventID:       "evt-123",
		EventType:     "user.registered",
		EventVersion:  1,
		OccurredAt:    time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
		TraceID:       "trace-abc",
		Producer:      "auth-service",
		AggregateType: "user",
		AggregateID:   "agg-789",
		Actor:         Actor{UserID: "user-1", Type: "user"},
		Payload:       json.RawMessage(`{"email":"alice@teamboard.local"}`),
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify the canonical wire field names are present.
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	for _, key := range []string{"event_id", "event_type", "event_version", "occurred_at", "producer", "aggregate_type", "aggregate_id", "actor", "payload"} {
		if _, ok := asMap[key]; !ok {
			t.Errorf("expected wire field %q in marshalled envelope", key)
		}
	}

	var out Envelope
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.EventID != in.EventID || out.EventType != in.EventType || out.EventVersion != in.EventVersion {
		t.Errorf("scalar fields not preserved: %+v", out)
	}
	if !out.OccurredAt.Equal(in.OccurredAt) {
		t.Errorf("occurred_at not preserved: got %v want %v", out.OccurredAt, in.OccurredAt)
	}
	if out.Actor != in.Actor {
		t.Errorf("actor not preserved: got %+v want %+v", out.Actor, in.Actor)
	}
	if string(out.Payload) != string(in.Payload) {
		t.Errorf("payload not preserved: got %s want %s", out.Payload, in.Payload)
	}
}

func TestEnvelope_OmitsEmptyTraceID(t *testing.T) {
	raw, err := json.Marshal(Envelope{EventType: "x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := asMap["trace_id"]; ok {
		t.Error("empty trace_id should be omitted from the wire envelope")
	}
}

func TestMustMarshalPayload(t *testing.T) {
	t.Run("marshals a value", func(t *testing.T) {
		got := MustMarshalPayload(map[string]string{"k": "v"})
		if string(got) != `{"k":"v"}` {
			t.Errorf("got %s", got)
		}
	})

	t.Run("panics on unmarshalable value", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic on unmarshalable payload")
			}
		}()
		MustMarshalPayload(make(chan int)) // channels cannot be JSON-encoded
	})
}

func TestIdempotencyFuncs_Delegates(t *testing.T) {
	var (
		hasID  string
		markID string
	)
	f := IdempotencyFuncs{
		Has: func(_ context.Context, id string) (bool, error) {
			hasID = id
			return true, nil
		},
		Mark: func(_ context.Context, id string) error {
			markID = id
			return nil
		},
	}

	ok, err := f.HasProcessed(context.Background(), "a")
	if err != nil || !ok || hasID != "a" {
		t.Errorf("HasProcessed delegation failed: ok=%v err=%v id=%q", ok, err, hasID)
	}
	if err := f.MarkProcessed(context.Background(), "b"); err != nil || markID != "b" {
		t.Errorf("MarkProcessed delegation failed: err=%v id=%q", err, markID)
	}
}

// fakeStore is a configurable IdempotencyStore for the IdempotentHandler tests.
type fakeStore struct {
	mu       sync.Mutex
	seen     map[string]bool
	hasErr   error
	markErr  error
	markCall int
}

func newFakeStore() *fakeStore { return &fakeStore{seen: map[string]bool{}} }

func (s *fakeStore) HasProcessed(_ context.Context, id string) (bool, error) {
	if s.hasErr != nil {
		return false, s.hasErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[id], nil
}

func (s *fakeStore) MarkProcessed(_ context.Context, id string) error {
	if s.markErr != nil {
		return s.markErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markCall++
	s.seen[id] = true
	return nil
}

func TestIdempotentHandler(t *testing.T) {
	env := Envelope{EventID: "evt-1", EventType: "task.created"}

	t.Run("runs inner and marks a fresh event", func(t *testing.T) {
		store := newFakeStore()
		var calls int
		h := IdempotentHandler(func(context.Context, Envelope) error { calls++; return nil }, store)

		if err := h(context.Background(), env); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("inner calls = %d, want 1", calls)
		}
		if store.markCall != 1 {
			t.Errorf("mark calls = %d, want 1", store.markCall)
		}
	})

	t.Run("skips a duplicate without running inner", func(t *testing.T) {
		store := newFakeStore()
		store.seen[env.EventID] = true
		var calls int
		h := IdempotentHandler(func(context.Context, Envelope) error { calls++; return nil }, store)

		if err := h(context.Background(), env); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 0 {
			t.Errorf("inner must not run for duplicate, got %d calls", calls)
		}
		if store.markCall != 0 {
			t.Errorf("duplicate must not be re-marked, got %d", store.markCall)
		}
	})

	t.Run("does not mark when inner fails", func(t *testing.T) {
		store := newFakeStore()
		wantErr := errors.New("handler boom")
		h := IdempotentHandler(func(context.Context, Envelope) error { return wantErr }, store)

		if err := h(context.Background(), env); !errors.Is(err, wantErr) {
			t.Fatalf("got %v, want %v", err, wantErr)
		}
		if store.markCall != 0 {
			t.Errorf("event must not be marked when inner fails, got %d", store.markCall)
		}
	})

	t.Run("propagates store lookup error and skips inner", func(t *testing.T) {
		store := newFakeStore()
		store.hasErr = errors.New("db down")
		var calls int
		h := IdempotentHandler(func(context.Context, Envelope) error { calls++; return nil }, store)

		if err := h(context.Background(), env); err == nil {
			t.Fatal("expected error from store lookup")
		}
		if calls != 0 {
			t.Errorf("inner must not run when lookup errors, got %d calls", calls)
		}
	})
}

// ─── Live-broker integration tests ───────────────────────────────────────────
//
// These exercise the real publisher-confirm + consumer round-trip against a
// running RabbitMQ. They are skipped under -short and skipped (not failed) when
// no broker is reachable, so unit runs and CI without a broker stay green.
// Point them at a broker via TEST_RABBITMQ_URL; defaults to the local dev stack.

const (
	testBrokerEnv        = "TEST_RABBITMQ_URL"
	defaultTestBrokerURL = "amqp://teamboard:teamboard@localhost:5672/"
)

func dialTestBroker(t *testing.T) *amqp.Connection {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live-broker test in -short mode")
	}
	url := os.Getenv(testBrokerEnv)
	if url == "" {
		url = defaultTestBrokerURL
	}
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(2 * time.Second)})
	if err != nil {
		t.Skipf("rabbitmq not reachable at %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// declareTestTopology returns isolated exchange/queue names and tears them down.
func newTestTopology(t *testing.T, conn *amqp.Connection) TopologyOptions {
	t.Helper()
	suffix := uuid.NewString()
	opts := TopologyOptions{
		Exchange:    "teamboard.test." + suffix,
		Queue:       "teamboard.test.q." + suffix,
		BindingKeys: []string{"test.#"},
	}
	t.Cleanup(func() {
		ch, err := conn.Channel()
		if err != nil {
			return
		}
		defer ch.Close()
		_, _ = ch.QueueDelete(opts.Queue, false, false, false)
		_ = ch.ExchangeDelete(opts.Exchange, false, false)
		_ = ch.ExchangeDelete(opts.Exchange+DefaultDLXSuffix, false, false)
	})
	return opts
}

func TestPublisher_ConfirmAndConsumerRoundTrip(t *testing.T) {
	conn := dialTestBroker(t)
	opts := newTestTopology(t, conn)

	cons, err := NewConsumer(conn, opts)
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}
	t.Cleanup(func() { _ = cons.Close() })

	pub, err := NewPublisher(conn, opts.Exchange)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	t.Cleanup(func() { _ = pub.Close() })

	received := make(chan Envelope, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		_ = cons.Subscribe(ctx, func(_ context.Context, env Envelope) error {
			received <- env
			return nil
		})
	}()

	want := Envelope{
		EventID:       uuid.NewString(),
		EventType:     "test.thing.happened",
		EventVersion:  1,
		OccurredAt:    time.Now().UTC().Truncate(time.Millisecond),
		Producer:      "eventbus-test",
		AggregateType: "thing",
		AggregateID:   uuid.NewString(),
		Actor:         Actor{Type: "service"},
		Payload:       json.RawMessage(`{"hello":"world"}`),
	}

	pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pubCancel()
	// Publish must only return nil after the broker confirms the message.
	if err := pub.Publish(pubCtx, want); err != nil {
		t.Fatalf("publish (with confirm) failed: %v", err)
	}

	select {
	case got := <-received:
		if got.EventID != want.EventID || got.EventType != want.EventType {
			t.Errorf("identity mismatch: got %s/%s want %s/%s", got.EventID, got.EventType, want.EventID, want.EventType)
		}
		if string(got.Payload) != string(want.Payload) {
			t.Errorf("payload mismatch: got %s want %s", got.Payload, want.Payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the published event to be consumed")
	}
}

func TestIdempotentHandler_OverBroker_DeliversOnce(t *testing.T) {
	conn := dialTestBroker(t)
	opts := newTestTopology(t, conn)

	cons, err := NewConsumer(conn, opts)
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}
	t.Cleanup(func() { _ = cons.Close() })

	pub, err := NewPublisher(conn, opts.Exchange)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	t.Cleanup(func() { _ = pub.Close() })

	store := &countingStore{seen: map[string]bool{}}
	var innerCalls int32
	var mu sync.Mutex
	handler := IdempotentHandler(func(context.Context, Envelope) error {
		mu.Lock()
		innerCalls++
		mu.Unlock()
		return nil
	}, store)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = cons.Subscribe(ctx, handler) }()

	env := Envelope{
		EventID:      uuid.NewString(),
		EventType:    "test.duplicate",
		EventVersion: 1,
		OccurredAt:   time.Now().UTC(),
		Producer:     "eventbus-test",
		Actor:        Actor{Type: "service"},
		Payload:      json.RawMessage(`{}`),
	}

	// Publish the same event_id twice; both are delivered, only one runs inner.
	for i := 0; i < 2; i++ {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := pub.Publish(pubCtx, env); err != nil {
			pubCancel()
			t.Fatalf("publish %d: %v", i, err)
		}
		pubCancel()
	}

	// Wait until both deliveries have hit the idempotency check.
	deadline := time.After(5 * time.Second)
	for store.hasCount() < 2 {
		select {
		case <-deadline:
			t.Fatalf("only %d of 2 deliveries reached the idempotency store", store.hasCount())
		case <-time.After(20 * time.Millisecond):
		}
	}
	// Give the second (skipped) delivery a moment to settle, then assert.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	got := innerCalls
	mu.Unlock()
	if got != 1 {
		t.Errorf("inner handler ran %d times, want exactly 1 (idempotency)", got)
	}
}

// countingStore is an in-memory IdempotencyStore that counts HasProcessed calls,
// used to assert both broker deliveries reached the dedup gate.
type countingStore struct {
	mu       sync.Mutex
	seen     map[string]bool
	hasCalls int
}

func (s *countingStore) HasProcessed(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasCalls++
	return s.seen[id], nil
}

func (s *countingStore) MarkProcessed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[id] = true
	return nil
}

func (s *countingStore) hasCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasCalls
}
