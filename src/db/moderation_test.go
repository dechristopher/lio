package db

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestBanStateDecoding locks how the single banned_until column encodes all
// three states. Getting this wrong in either direction is a real incident: a
// permanent ban decoding as unbanned lets a sanctioned account back in, and an
// expired ban decoding as banned locks out someone whose sanction has served
// its term.
func TestBanStateDecoding(t *testing.T) {
	reason := "engine assistance"
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)

	t.Run("null is not banned", func(t *testing.T) {
		got := banFrom(pgtype.Timestamptz{}, &reason)
		if got.Banned || got.Permanent {
			t.Fatalf("NULL banned_until decoded as %+v", got)
		}
	})

	t.Run("infinity is a permanent ban", func(t *testing.T) {
		got := banFrom(pgtype.Timestamptz{
			InfinityModifier: pgtype.Infinity, Valid: true,
		}, &reason)
		if !got.Banned || !got.Permanent {
			t.Fatalf("infinity decoded as %+v", got)
		}
		if !got.Until.IsZero() {
			t.Errorf("permanent ban carries an expiry: %v", got.Until)
		}
		if got.Reason != reason {
			t.Errorf("reason = %q, want %q", got.Reason, reason)
		}
	})

	t.Run("future expiry is in force", func(t *testing.T) {
		got := banFrom(pgtype.Timestamptz{Time: future, Valid: true}, &reason)
		if !got.Banned || got.Permanent {
			t.Fatalf("future expiry decoded as %+v", got)
		}
		if !got.Until.Equal(future) {
			t.Errorf("Until = %v, want %v", got.Until, future)
		}
	})

	t.Run("past expiry has lapsed", func(t *testing.T) {
		got := banFrom(pgtype.Timestamptz{Time: past, Valid: true}, &reason)
		if got.Banned {
			t.Fatalf("expired ban still in force: %+v", got)
		}
	})

	t.Run("negative infinity fails closed", func(t *testing.T) {
		got := banFrom(pgtype.Timestamptz{
			InfinityModifier: pgtype.NegativeInfinity, Valid: true,
		}, &reason)
		if got.Banned {
			t.Fatalf("-infinity decoded as banned: %+v", got)
		}
	})

	t.Run("nil reason is empty", func(t *testing.T) {
		got := banFrom(pgtype.Timestamptz{
			InfinityModifier: pgtype.Infinity, Valid: true,
		}, nil)
		if got.Reason != "" {
			t.Errorf("reason = %q, want empty", got.Reason)
		}
	})
}

// TestBanUntilEncoding is the write-side counterpart: a zero time means
// permanent, anything else a dated expiry. Round-trips through banFrom.
func TestBanUntilEncoding(t *testing.T) {
	perm := banUntil(time.Time{})
	if perm.InfinityModifier != pgtype.Infinity || !perm.Valid {
		t.Fatalf("zero time encoded as %+v, want valid infinity", perm)
	}
	if got := banFrom(perm, nil); !got.Banned || !got.Permanent {
		t.Errorf("permanent ban did not round-trip: %+v", got)
	}

	until := time.Now().Add(time.Hour)
	temp := banUntil(until)
	if temp.InfinityModifier != pgtype.Finite || !temp.Valid || !temp.Time.Equal(until) {
		t.Fatalf("dated expiry encoded as %+v", temp)
	}
	if got := banFrom(temp, nil); !got.Banned || got.Permanent {
		t.Errorf("temporary ban did not round-trip: %+v", got)
	}
}

// TestValidCategory locks the report categories against the CHECK constraint
// they mirror: the handler validates with this, so a category it accepts that
// the database rejects would be a 500 on an ordinary player's report.
func TestValidCategory(t *testing.T) {
	for _, c := range ReportCategories {
		if !ValidCategory(c) {
			t.Errorf("ValidCategory(%q) = false for a listed category", c)
		}
	}
	for _, bad := range []string{"", "CHEATING", "spam", "'; drop table reports; --"} {
		if ValidCategory(bad) {
			t.Errorf("ValidCategory(%q) = true", bad)
		}
	}
}
