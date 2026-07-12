package room

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// snapshotStore is the storage backend for room snapshots (restart
// persistence, arch/STATE_PERSISTENCE_SCALING.md). Implemented by
// cache.RoomSnapshots (Redis, production) and by the in-memory fake in the
// persister tests; the room package itself stays storage-agnostic.
type snapshotStore interface {
	// PutRooms writes a batch of snapshots, each with the given TTL. A batch
	// is one persister tick's dirty set — implementations should write it in
	// one round trip (pipeline) where they can.
	PutRooms(snaps map[string][]byte, ttl time.Duration) error
	// DeleteRoom removes a room's snapshot (room closed).
	DeleteRoom(id string) error
	// LoadRooms returns every stored snapshot keyed by room id (boot).
	LoadRooms() (map[string][]byte, error)
}

const (
	// persistTick is the write-behind coalescing window: how often the dirty
	// set is captured and flushed. Worst-case loss on a hard crash (not a
	// drain, which flushes synchronously) is one tick of moves.
	persistTick = 250 * time.Millisecond

	// sweepInterval re-marks every live room so an idle-but-alive room (an
	// untimed casual game, a long think) keeps refreshing its snapshot TTL.
	// Also self-heals any missed write within one sweep.
	sweepInterval = 15 * time.Minute

	// snapshotTTL bounds how long a snapshot outlives its last write; live
	// rooms refresh it every write and at latest every sweep. It is a
	// belt-and-suspenders bound, not a lifecycle mechanism — room teardown
	// deletes the snapshot explicitly.
	snapshotTTL = 24 * time.Hour

	// maxSnapshotAge is the rehydration staleness cutoff: snapshots older than
	// this at boot are dropped (their players are long gone) rather than
	// restored. With the sweep refreshing live rooms' savedAt, only rooms from
	// a process that has been down this long can trip it.
	maxSnapshotAge = 24 * time.Hour
)

// persister owns the write-behind snapshot flow: room routines mark
// themselves dirty (markDirty) and a single goroutine coalesces, captures
// (Persist), and writes batches to the store. Deletions ride the same loop so
// a room's teardown DELETE can never be overtaken by an in-flight write of
// its own snapshot.
type persister struct {
	store snapshotStore

	mu      sync.Mutex
	dirty   map[string]*Instance
	deleted map[string]struct{}

	quit chan struct{}
}

// activePersister is the process-wide persister, set once at boot before any
// room exists (UpPersister) and read lock-free by the mark/forget hooks on
// every room routine. Atomic so tests can install and tear down persisters
// without racing leaked room routines from earlier tests.
var activePersister atomic.Pointer[persister]

// UpPersister starts the write-behind snapshot persister against the given
// store. Called once at boot, after rehydration and before the server begins
// accepting connections. A nil-store or repeat call is a programmer error and
// simply replaces the previous persister.
func UpPersister(s snapshotStore) {
	p := newPersister(s)
	activePersister.Store(p)
	go p.run()
	util.Debug(str.CRoom, "room snapshot persister online")
}

func newPersister(s snapshotStore) *persister {
	return &persister{
		store:   s,
		dirty:   make(map[string]*Instance),
		deleted: make(map[string]struct{}),
		quit:    make(chan struct{}),
	}
}

// markDirty enqueues the room for the persister's next flush. Called from the
// room routine at every state transition, applied move, and handled control;
// a no-op when persistence is not configured (local dev without Redis).
func markDirty(r *Instance) {
	if p := activePersister.Load(); p != nil {
		p.mark(r)
	}
}

// forgetSnapshot enqueues deletion of the room's snapshot (room teardown).
// Ordered through the persister loop so it serializes after any in-flight
// write of the same room.
func forgetSnapshot(roomID string) {
	if p := activePersister.Load(); p != nil {
		p.forget(roomID)
	}
}

// FlushSnapshots synchronously persists every live room and processes pending
// deletions: the drain hook (called on shutdown after inbound mutations are
// gated) and a test seam. No-op without an active persister.
func FlushSnapshots() {
	p := activePersister.Load()
	if p == nil {
		return
	}
	rooms.Range(func(_, v interface{}) bool {
		p.mark(v.(*Instance))
		return true
	})
	p.flush()
}

// RehydrateAll restores every persisted room from the store at boot: stale and
// unreadable snapshots are dropped (and deleted), the rest are rebuilt and
// their routines started. Returns the number of rooms restored. Must complete
// before the HTTP listener accepts connections, or reconnecting clients race
// the restore and get bounced as "room gone".
func RehydrateAll(s snapshotStore) int {
	snaps, err := s.LoadRooms()
	if err != nil {
		util.Error(str.CRoom, "rehydration: loading snapshots failed: %v", err)
		return 0
	}

	restored := 0
	for id, data := range snaps {
		// staleness check on the envelope alone, so a snapshot too old to be
		// worth restoring is dropped without a full rebuild
		var envelope struct {
			At time.Time `json:"at"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil ||
			time.Since(envelope.At) > maxSnapshotAge {
			util.Info(str.CRoom, "[%s] dropping stale/unreadable snapshot", id)
			_ = s.DeleteRoom(id)
			continue
		}

		r, err := Rehydrate(data)
		if err != nil {
			util.Error(str.CRoom, "[%s] rehydration failed, dropping snapshot: %v", id, err)
			_ = s.DeleteRoom(id)
			continue
		}
		if err := r.StartRehydrated(); err != nil {
			util.Error(str.CRoom, "[%s] rehydrated room failed to start: %v", id, err)
			continue
		}
		restored++
	}

	if restored > 0 {
		util.Info(str.CRoom, "restored %d room(s) from snapshots", restored)
	}
	return restored
}

func (p *persister) mark(r *Instance) {
	p.mu.Lock()
	p.dirty[r.ID] = r
	p.mu.Unlock()
}

func (p *persister) forget(roomID string) {
	p.mu.Lock()
	delete(p.dirty, roomID)
	p.deleted[roomID] = struct{}{}
	p.mu.Unlock()
}

// run is the persister loop: flush the dirty set every tick, and periodically
// re-mark all live rooms so idle rooms keep their snapshot TTLs fresh.
func (p *persister) run() {
	tick := time.NewTicker(persistTick)
	defer tick.Stop()
	sweep := time.NewTicker(sweepInterval)
	defer sweep.Stop()

	for {
		select {
		case <-p.quit:
			return
		case <-tick.C:
			p.flush()
		case <-sweep.C:
			rooms.Range(func(_, v interface{}) bool {
				p.mark(v.(*Instance))
				return true
			})
		}
	}
}

// flush captures and writes the current dirty set and processes pending
// deletions. Failed writes are re-marked and retried on a later tick, so a
// transient Redis outage degrades to a longer coalescing window instead of
// lost persistence.
func (p *persister) flush() {
	p.mu.Lock()
	dirty := p.dirty
	deleted := p.deleted
	p.dirty = make(map[string]*Instance)
	p.deleted = make(map[string]struct{})
	p.mu.Unlock()

	if len(dirty) > 0 {
		batch := make(map[string][]byte, len(dirty))
		for id, r := range dirty {
			// a room can be torn down between mark and flush; its deletion is
			// either already in this batch's deleted set or will arrive later
			if _, gone := deleted[id]; gone {
				continue
			}
			if data, ok := r.Persist(); ok {
				batch[id] = data
			}
		}
		if len(batch) > 0 {
			if err := p.store.PutRooms(batch, snapshotTTL); err != nil {
				util.Error(str.CRoom, "snapshot write failed (%d rooms), will retry: %v", len(batch), err)
				p.mu.Lock()
				for id, r := range dirty {
					if _, gone := p.deleted[id]; !gone {
						p.dirty[id] = r
					}
				}
				p.mu.Unlock()
			}
		}
	}

	for id := range deleted {
		if err := p.store.DeleteRoom(id); err != nil {
			util.Error(str.CRoom, "[%s] snapshot delete failed, will retry: %v", id, err)
			p.mu.Lock()
			p.deleted[id] = struct{}{}
			p.mu.Unlock()
		}
	}
}

// stop terminates the persister loop (tests).
func (p *persister) stop() {
	close(p.quit)
}
