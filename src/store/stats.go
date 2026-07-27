package store

import (
	"context"
	"sync/atomic"
	"time"
)

// Operational stats for the /system console: whether the PGN bucket is
// reachable, and how the archive writes into it have actually been going.
//
// The counters matter more here than anywhere else in the stack. A PGN write
// failure is logged and then dropped — the game is already over and the
// relational archive has its own copy — so nothing else in the system will ever
// tell an operator that object archival has been quietly failing for a week.

// probeTimeout bounds the liveness probe. The object store is the one backend
// that may legitimately live off-network (a remote S3 endpoint), so this is
// looser than the loopback-network probes in db/cache while still being far
// too short to hang a page on.
const probeTimeout = 3 * time.Second

// Write/read counters since boot, plus the most recent failure.
var (
	putOK    atomic.Int64
	putFail  atomic.Int64
	getOK    atomic.Int64
	getFail  atomic.Int64
	lastFail atomic.Pointer[Failure]
)

// Failure is the most recent object-store error, kept for display.
type Failure struct {
	When time.Time
	Op   string // "put" or "get"
	Key  string
	Err  string
}

// note records one object operation's outcome.
func note(op, key string, err error, ok, fail *atomic.Int64) error {
	if err == nil {
		ok.Add(1)
		return nil
	}
	fail.Add(1)
	lastFail.Store(&Failure{When: time.Now(), Op: op, Key: key, Err: err.Error()})
	return err
}

// Stats is the object store's operational picture.
type Stats struct {
	// Configured is whether an endpoint was supplied at all; Reachable whether
	// the bucket probe just succeeded. Local dev routinely runs as the former
	// without the latter and must not read as an outage.
	Configured bool
	Reachable  bool
	Endpoint   string
	Bucket     string
	Latency    time.Duration
	Err        string

	PutOK       int64
	PutFail     int64
	GetOK       int64
	GetFail     int64
	LastFailure *Failure
}

// GetStats samples the object store. The probe is BucketExists rather than a
// bucket listing: it is a single HEAD against the exact bucket lio writes to,
// so it proves credentials, network and target in one cheap call without
// enumerating an archive that grows without bound.
func GetStats() Stats {
	s := Stats{
		Configured:  C != nil,
		Endpoint:    objectStoreEndpoint,
		Bucket:      string(PGNBucket),
		PutOK:       putOK.Load(),
		PutFail:     putFail.Load(),
		GetOK:       getOK.Load(),
		GetFail:     getFail.Load(),
		LastFailure: lastFail.Load(),
	}
	if !s.Configured {
		return s
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	start := time.Now()
	exists, err := C.BucketExists(ctx, string(PGNBucket))
	s.Latency = time.Since(start)
	switch {
	case err != nil:
		s.Err = err.Error()
	case !exists:
		// reachable and authenticated, but pointed at a bucket that is not
		// there — a misconfiguration that would otherwise only surface as a
		// failed archive write after the next finished game
		s.Err = "bucket " + string(PGNBucket) + " does not exist"
	default:
		s.Reachable = true
	}
	return s
}
