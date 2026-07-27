package db

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Operational stats for the /system console: the connection pool's own
// accounting, a cheap catalog read describing what is stored, and the
// process-local archive write counters.
//
// The pool figures are free (in-memory counters pgxpool already keeps). The
// catalog figures cost one round trip against pg_stat_user_tables, which reads
// the statistics collector rather than the tables — no sequential scan, so it
// stays cheap as the archive grows. Row counts are therefore *estimates*, which
// is the right trade here: an operator wants to know the moves table has ~4
// million rows, not to pay a full count to learn it has 4,127,338.

// probeTimeout bounds the liveness probe behind the console panel. Deliberately
// far tighter than opTimeout: the panel's job during an incident is to say
// "unreachable" quickly, not to sit for five seconds proving it.
const probeTimeout = time.Second

// archiveOK / archiveFail count game archive writes since boot, and lastErr
// holds the most recent failure. These answer the question the console panel
// exists for — "is archiving actually working right now?" — which pool health
// alone cannot: a reachable database that rejects every write looks perfect
// from the pool's point of view.
var (
	archiveOK   atomic.Int64
	archiveFail atomic.Int64
	lastErr     atomic.Pointer[ArchiveFailure]
)

// ArchiveFailure is the most recent archive write error, kept for display.
type ArchiveFailure struct {
	When time.Time
	Err  string
}

// noteArchive records the outcome of one archive write.
func noteArchive(err error) {
	if err == nil {
		archiveOK.Add(1)
		return
	}
	archiveFail.Add(1)
	lastErr.Store(&ArchiveFailure{When: time.Now(), Err: err.Error()})
}

// Stats is the database's operational picture.
type Stats struct {
	// Configured is whether a DSN was supplied at all; Reachable whether the
	// probe below just succeeded. The pair distinguishes "no archive by design"
	// (local dev) from "archive is down", which must never render the same way.
	Configured bool
	Reachable  bool
	Latency    time.Duration
	Err        string

	// Pool accounting.
	MaxConns          int32
	TotalConns        int32
	IdleConns         int32
	AcquiredConns     int32
	ConstructingConns int32
	AcquireCount      int64
	AcquireDuration   time.Duration
	// EmptyAcquireCount is acquires that had to wait for a connection, and
	// EmptyAcquireWait the total time spent waiting. A climbing pair here is the
	// signal to raise pool_max_conns.
	EmptyAcquireCount    int64
	EmptyAcquireWait     time.Duration
	CanceledAcquireCount int64
	NewConnsCount        int64

	// Storage.
	DatabaseBytes int64
	Tables        []TableStat

	// Archive writes since boot.
	ArchiveOK   int64
	ArchiveFail int64
	LastFailure *ArchiveFailure
}

// TableStat is one table's estimated size, biggest first in Stats.Tables.
type TableStat struct {
	Name string
	// Rows is the planner's live-tuple estimate, not an exact count.
	Rows int64
	// Bytes includes indexes and TOAST — what the table actually costs on disk.
	Bytes int64
}

// tablesShown bounds the per-table breakdown. The archive has ~14 tables and
// the interesting ones are always the largest few; the rest are rounding error
// against the database total shown beside them.
const tablesShown = 6

// GetStats samples the database. It never returns an error: an unreachable
// database is a *result* this panel exists to display, not a failure that
// should take the console page down with it.
func GetStats() Stats {
	s := Stats{
		Configured:  Pool != nil,
		ArchiveOK:   archiveOK.Load(),
		ArchiveFail: archiveFail.Load(),
		LastFailure: lastErr.Load(),
	}
	if !s.Configured {
		return s
	}

	poolStats(&s, Pool.Stat())

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	start := time.Now()
	if err := Pool.Ping(ctx); err != nil {
		s.Latency = time.Since(start)
		s.Err = err.Error()
		return s
	}
	s.Latency = time.Since(start)
	s.Reachable = true

	// a catalog read that fails leaves the storage figures zero rather than
	// discarding the pool + liveness detail already gathered
	if err := catalogStats(ctx, &s); err != nil {
		s.Err = err.Error()
	}
	return s
}

// poolStats copies the pool's counters into the sample.
func poolStats(s *Stats, p *pgxpool.Stat) {
	s.MaxConns = p.MaxConns()
	s.TotalConns = p.TotalConns()
	s.IdleConns = p.IdleConns()
	s.AcquiredConns = p.AcquiredConns()
	s.ConstructingConns = p.ConstructingConns()
	s.AcquireCount = p.AcquireCount()
	s.AcquireDuration = p.AcquireDuration()
	s.EmptyAcquireCount = p.EmptyAcquireCount()
	s.EmptyAcquireWait = p.EmptyAcquireWaitTime()
	s.CanceledAcquireCount = p.CanceledAcquireCount()
	s.NewConnsCount = p.NewConnsCount()
}

// catalogStats reads the database size and the largest tables.
//
// These are raw pool queries rather than sqlc-generated ones on purpose: sqlc
// types queries against the migration schema, and the system catalogs are not
// in it. Hand-writing two constant, parameterless SELECTs is honest; teaching
// sqlc about pg_catalog to generate them would not be.
func catalogStats(ctx context.Context, s *Stats) error {
	if err := Pool.QueryRow(ctx,
		`SELECT pg_database_size(current_database())`).Scan(&s.DatabaseBytes); err != nil {
		return err
	}

	rows, err := Pool.Query(ctx, `
		SELECT relname,
		       n_live_tup,
		       pg_total_relation_size(relid)
		  FROM pg_stat_user_tables
		 ORDER BY pg_total_relation_size(relid) DESC
		 LIMIT $1`, tablesShown)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var t TableStat
		if err := rows.Scan(&t.Name, &t.Rows, &t.Bytes); err != nil {
			return err
		}
		s.Tables = append(s.Tables, t)
	}
	return rows.Err()
}
