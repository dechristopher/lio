package room

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory snapshotStore for persister tests.
type memStore struct {
	mu       sync.Mutex
	snaps    map[string][]byte
	failPuts bool
}

func newMemStore() *memStore {
	return &memStore{snaps: make(map[string][]byte)}
}

func (m *memStore) PutRooms(snaps map[string][]byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failPuts {
		return errors.New("memstore: simulated outage")
	}
	for id, data := range snaps {
		m.snaps[id] = data
	}
	return nil
}

func (m *memStore) DeleteRoom(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.snaps, id)
	return nil
}

func (m *memStore) LoadRooms() (map[string][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string][]byte, len(m.snaps))
	for id, data := range m.snaps {
		out[id] = data
	}
	return out, nil
}

func (m *memStore) get(id string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.snaps[id]
	return data, ok
}

// installPersister wires a fresh persister (no run loop — tests drive flush
// directly for determinism) into the global hook and tears it down after.
func installPersister(t *testing.T, ms *memStore) *persister {
	t.Helper()
	p := newPersister(ms)
	activePersister.Store(p)
	t.Cleanup(func() { activePersister.Store(nil) })
	return p
}

// TestPersisterWritesDirtyRooms drives the real trigger path: makeMove marks
// the room dirty, a flush captures and writes it, and a teardown forget
// deletes it.
func TestPersisterWritesDirtyRooms(t *testing.T) {
	ms := newMemStore()
	p := installPersister(t, ms)

	r := newTestInstance(t, "wp", "bp")
	r.ID = "persistwrite"
	driveToOngoing(t, r)
	r.game.Clock.Start()
	defer r.game.Clock.Stop(false, true)

	// the move path itself marks dirty
	playTestMoves(t, r, 1)
	p.flush()

	data, ok := ms.get(r.ID)
	if !ok {
		t.Fatal("move did not produce a persisted snapshot")
	}
	var snap PersistedRoom
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("stored snapshot unreadable: %v", err)
	}
	if len(snap.Moves) != 1 {
		t.Fatalf("stored snapshot has %d moves, want 1", len(snap.Moves))
	}

	// teardown deletes the snapshot, serialized through the same loop
	forgetSnapshot(r.ID)
	p.flush()
	if _, ok := ms.get(r.ID); ok {
		t.Fatal("snapshot survived room teardown")
	}
}

// TestPersisterRetriesFailedWrites: a failed batch is re-marked and lands on
// a later flush once the store recovers (transient Redis outage tolerance).
func TestPersisterRetriesFailedWrites(t *testing.T) {
	ms := newMemStore()
	p := installPersister(t, ms)

	r := newTestInstance(t, "wp", "bp")
	r.ID = "persistretry"
	driveToOngoing(t, r)

	ms.mu.Lock()
	ms.failPuts = true
	ms.mu.Unlock()

	p.mark(r)
	p.flush()
	if _, ok := ms.get(r.ID); ok {
		t.Fatal("write unexpectedly succeeded during simulated outage")
	}

	ms.mu.Lock()
	ms.failPuts = false
	ms.mu.Unlock()

	// the failed batch was re-marked; the next flush writes it
	p.flush()
	if _, ok := ms.get(r.ID); !ok {
		t.Fatal("failed write was not retried after the store recovered")
	}
}

// TestPersisterForgetWinsOverMark: a room marked dirty and then torn down in
// the same window is deleted, not resurrected by its own pending write.
func TestPersisterForgetWinsOverMark(t *testing.T) {
	ms := newMemStore()
	p := installPersister(t, ms)

	r := newTestInstance(t, "wp", "bp")
	r.ID = "persistforget"
	driveToOngoing(t, r)

	p.mark(r)
	p.forget(r.ID)
	p.flush()

	if _, ok := ms.get(r.ID); ok {
		t.Fatal("torn-down room's pending write resurrected its snapshot")
	}
}

// TestRehydrateAllRestoresAndPrunes: boot rehydration restores valid
// snapshots, and drops (deleting from the store) corrupt and stale ones.
func TestRehydrateAllRestoresAndPrunes(t *testing.T) {
	ms := newMemStore()

	// valid ongoing room snapshot
	r := newTestInstance(t, "wp", "bp")
	r.ID = "rehydrateall"
	driveToOngoing(t, r)
	r.game.Clock.Start()
	playTestMoves(t, r, 1)
	valid, ok := r.Persist()
	if !ok {
		t.Fatal("persist failed")
	}
	r.game.Clock.Stop(false, true)

	// stale copy: same snapshot with savedAt pushed past the cutoff
	var doctored map[string]interface{}
	if err := json.Unmarshal(valid, &doctored); err != nil {
		t.Fatal(err)
	}
	doctored["at"] = time.Now().Add(-2 * maxSnapshotAge)
	doctored["id"] = "rehydratestale"
	stale, err := json.Marshal(doctored)
	if err != nil {
		t.Fatal(err)
	}

	ms.snaps["rehydrateall"] = valid
	ms.snaps["rehydratestale"] = stale
	ms.snaps["rehydratejunk"] = []byte("{not json")

	restored := RehydrateAll(ms)
	t.Cleanup(func() { rooms.Delete("rehydrateall") })

	if restored != 1 {
		t.Fatalf("restored %d rooms, want 1", restored)
	}
	if _, err := Get("rehydrateall"); err != nil {
		t.Fatal("valid snapshot was not restored into the room registry")
	}
	if _, ok := ms.get("rehydratestale"); ok {
		t.Fatal("stale snapshot was not pruned from the store")
	}
	if _, ok := ms.get("rehydratejunk"); ok {
		t.Fatal("corrupt snapshot was not pruned from the store")
	}
}
