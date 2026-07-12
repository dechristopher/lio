package cache

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// C is the Redis client instance, nil when no cache is configured (local dev
// without lio_redis_addr — room persistence is then disabled, see room.Up*).
var C *redis.Client

// opTimeout bounds every cache round trip: the cache lives on the loopback /
// compose network, so anything slower than this is an outage, not latency, and
// callers (the room snapshot persister) must never be held hostage by it.
const opTimeout = 2 * time.Second

// Up brings the Redis connection online. Follows store.Up's degradation
// pattern: unset address in local dev is fine (warn and run without restart
// persistence); in prod a missing or unreachable Redis refuses to boot —
// silently serving without snapshots would mean deploys look safe while every
// live game is one restart from vanishing.
func Up() {
	addr := config.ReadSecretFallback("lio_redis_addr")

	if addr == "" {
		if env.IsLocal() {
			util.Info(str.CCache, "no redis configured; room persistence disabled (local)")
			return
		}
		log.Fatalln(str.CCache, "no redis address configured (lio_redis_addr)")
	}

	C = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: config.ReadSecretFallback("lio_redis_password"), // optional
	})

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	if err := C.Ping(ctx).Err(); err != nil {
		if env.IsLocal() {
			util.Error(str.CCache, "redis unreachable at %s; room persistence disabled (local): %v", addr, err)
			C = nil
			return
		}
		log.Fatalln(str.CCache, "redis unreachable:", err.Error())
	}

	util.Debug(str.CCache, "redis online at %s", addr)
}

// Ready reports whether a cache connection is configured and was reachable at
// boot. Runtime outages after a healthy boot do not flip this: writes fail and
// are retried on the next persister tick instead (see room/persister.go).
func Ready() bool {
	return C != nil
}

// Ctx returns a context bounded by the standard cache op timeout, and its
// cancel func.
func Ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}
