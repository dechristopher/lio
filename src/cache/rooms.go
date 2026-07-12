package cache

import (
	"errors"
	"strings"
	"time"
)

// roomKeyPrefix namespaces room snapshots in Redis. Layer 2 (multi-instance)
// will add sibling namespaces (owner leases, home digests) beside it.
const roomKeyPrefix = "lio:room:"

// errOffline is returned when no cache is configured; the room persister
// logs and retries, so a local dev boot without Redis stays quiet after the
// single bring-up notice.
var errOffline = errors.New("cache: not configured")

// RoomSnapshots is the Redis-backed room snapshot store; it satisfies the
// room package's snapshotStore interface (structurally — cache does not
// import room).
type RoomSnapshots struct{}

func roomKey(id string) string {
	return roomKeyPrefix + id
}

// PutRooms writes a batch of room snapshots in one pipelined round trip,
// each with the given TTL.
func (RoomSnapshots) PutRooms(snaps map[string][]byte, ttl time.Duration) error {
	if C == nil {
		return errOffline
	}
	ctx, cancel := Ctx()
	defer cancel()

	pipe := C.Pipeline()
	for id, data := range snaps {
		pipe.Set(ctx, roomKey(id), data, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// DeleteRoom removes a room's snapshot.
func (RoomSnapshots) DeleteRoom(id string) error {
	if C == nil {
		return errOffline
	}
	ctx, cancel := Ctx()
	defer cancel()
	return C.Del(ctx, roomKey(id)).Err()
}

// LoadRooms returns every stored room snapshot keyed by room id (boot
// rehydration). A key that disappears between scan and read (TTL expiry) is
// skipped, not an error.
func (RoomSnapshots) LoadRooms() (map[string][]byte, error) {
	if C == nil {
		return nil, errOffline
	}
	ctx, cancel := Ctx()
	defer cancel()

	snaps := make(map[string][]byte)
	iter := C.Scan(ctx, 0, roomKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		data, err := C.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		snaps[strings.TrimPrefix(key, roomKeyPrefix)] = data
	}
	return snaps, iter.Err()
}
