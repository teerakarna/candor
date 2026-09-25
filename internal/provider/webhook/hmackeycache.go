/*
Copyright 2026 Albert Asawaroengchai.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// hmacKeyCacheTTL bounds how long a resolved HMAC key is reused before the next request re-reads
// its Secret from the apiserver. Short enough that a rotated or deleted Secret takes effect
// quickly with no watch involved (a watch would need the same cluster-wide list/watch permission
// disabling the Secret cache was meant to avoid - see cmd/main.go's managerClientOptions); long
// enough to absorb a burst of requests against the same policy - valid or not - as one apiserver
// round trip's worth of load instead of one per request.
const hmacKeyCacheTTL = 20 * time.Second

type cachedHMACKey struct {
	value   []byte
	expires time.Time
}

// hmacKeyCache is a short-TTL, in-process cache of resolved WebhookReceiver Secret data, keyed by
// the Secret's namespace/name. Disabling the manager's Secret cache (issue #58) turned every
// request's Secret Get into an uncached, direct apiserver round trip, on exactly the path a flood
// of POSTs against a known namespace/policy route would hit hardest - including requests that
// ultimately fail signature verification, since the Secret has to be read before a signature can
// even be checked. TTL-based invalidation, not a watch: the whole point of disabling the cache
// was to stop needing cluster-wide list/watch on Secrets, so re-introducing a watch here to
// invalidate this cache would undo that.
//
// group coalesces concurrent cache misses for the same key (e.g. a burst of requests landing
// right after one entry's TTL expires) into a single resolve call - without it, N goroutines that
// all miss at the same moment would each hit the apiserver independently, undermining the point
// of caching at exactly the moment (a burst) it matters most.
//
// Zero value is ready to use - Receiver is built as a plain struct literal throughout this
// codebase and its tests, not through a constructor.
type hmacKeyCache struct {
	mu      sync.Mutex
	entries map[client.ObjectKey]cachedHMACKey
	group   singleflight.Group
}

func (c *hmacKeyCache) get(key client.ObjectKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expires) {
		// Deleted, not just ignored: otherwise every distinct key this cache ever sees (secret
		// rotations under a new name, renamed SignalPolicies) leaves a permanent stale entry for
		// the life of the process instead of the map staying bounded by keys currently in use.
		delete(c.entries, key)
		return nil, false
	}
	return entry.value, true
}

func (c *hmacKeyCache) set(key client.ObjectKey, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[client.ObjectKey]cachedHMACKey)
	}
	now := time.Now()
	// Swept here, not just lazily on a matching get(): a key that stops being queried entirely
	// (a deleted/renamed SignalPolicy, a Secret rotated under a new name) would otherwise never
	// hit get()'s own expiry check again and stay in the map for the life of the process. Sweeping
	// the whole map on every set (expected to stay small - one entry per distinct SignalPolicy
	// Secret actually receiving traffic) keeps it bounded by currently-active keys instead.
	for k, e := range c.entries {
		if now.After(e.expires) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = cachedHMACKey{value: value, expires: now.Add(hmacKeyCacheTTL)}
}

// getOrResolve returns the cached value for key, or calls resolve to obtain and cache one.
// Concurrent misses for the same key share a single resolve call (and its result, success or
// error) via group, rather than each independently hitting the apiserver - see group's doc
// comment above. A failed resolve is never cached (matching set's own caller in receiver.go),
// so a misconfigured Secret keeps being retried on the very next request rather than staying
// "stuck" for hmacKeyCacheTTL.
//
// ctx bounds only how long THIS caller waits for a result, not the shared resolve call itself
// (which runs to completion regardless, so it can still populate the cache for whoever asked
// next): DoChan, not Do, specifically so a caller whose own deadline is earlier than whichever
// request happens to be the singleflight leader's can give up on its own budget rather than being
// held hostage by another, unrelated request's timeline - Do blocks unconditionally until the
// shared call returns, with no way for one particular waiter to bail out early.
func (c *hmacKeyCache) getOrResolve(ctx context.Context, key client.ObjectKey, resolve func() ([]byte, error)) ([]byte, error) {
	if value, ok := c.get(key); ok {
		return value, nil
	}
	resultCh := c.group.DoChan(key.String(), func() (any, error) {
		// Re-checked here: another goroutine may have already populated the cache for this key
		// while this one was waiting to acquire the singleflight call, in which case there's
		// nothing left to resolve.
		if value, ok := c.get(key); ok {
			return value, nil
		}
		resolved, err := resolve()
		if err != nil {
			return nil, err
		}
		c.set(key, resolved)
		return resolved, nil
	})
	select {
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.([]byte), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
