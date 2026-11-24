// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"sync"
	"time"
)

// Cost line sets are shared by every caller and request for one minute: the
// summary, the transactions and both CSVs of the same range
// read the same set, so their totals agree.
const (
	costSetCacheSize = 8
	costSetCacheTTL  = 60 * time.Second
)

// costSetCache is a small LRU with a TTL. cache.Cache has no size bound, and
// a set can hold 80,000 lines, so it is not used here. It also shares one
// load between requests that ask for the same key at the same time.
type costSetCache struct {
	mu       sync.Mutex
	size     int
	ttl      time.Duration
	now      func() time.Time
	items    map[string]*costSetEntry
	order    []string // least recently used first
	inflight map[string]*costSetCall
}

type costSetEntry struct {
	set     *costSet
	expires time.Time
}

type costSetCall struct {
	done chan struct{}
	set  *costSet
	err  error
}

func newCostSetCache(size int, ttl time.Duration, now func() time.Time) *costSetCache {
	if now == nil {
		now = time.Now
	}
	return &costSetCache{
		size: size, ttl: ttl, now: now,
		items: map[string]*costSetEntry{}, inflight: map[string]*costSetCall{},
	}
}

func (c *costSetCache) touch(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.order = append(c.order, key)
}

// get returns the cached set for key, or runs load once for all callers that
// ask at the same time and caches its result (a failure is not cached). A
// caller that joins a load in flight stops waiting when its own ctx ends;
// the load goes on for the others.
func (c *costSetCache) get(ctx context.Context, key string, load func() (*costSet, error)) (*costSet, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok && c.now().Before(e.expires) {
		c.touch(key)
		c.mu.Unlock()
		return e.set, nil
	}
	if call, busy := c.inflight[key]; busy {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.set, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &costSetCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	call.set, call.err = load()

	c.mu.Lock()
	delete(c.inflight, key)
	if call.err == nil {
		c.items[key] = &costSetEntry{set: call.set, expires: c.now().Add(c.ttl)}
		c.touch(key)
		for len(c.order) > c.size {
			delete(c.items, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.mu.Unlock()
	close(call.done)
	return call.set, call.err
}
