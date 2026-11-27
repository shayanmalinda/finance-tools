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

import "sync"

// staleHold keeps the last result of a build whose rates are still being
// fetched, by key, until the fetch is done or a write clears it.
type staleHold[V any] struct {
	mu sync.Mutex
	m  map[string]V
}

func newStaleHold[V any]() *staleHold[V] { return &staleHold[V]{m: map[string]V{}} }

func (h *staleHold[V]) get(k string) (V, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.m[k]
	return v, ok
}

func (h *staleHold[V]) set(k string, v V) {
	h.mu.Lock()
	h.m[k] = v
	h.mu.Unlock()
}

func (h *staleHold[V]) del(k string) {
	h.mu.Lock()
	delete(h.m, k)
	h.mu.Unlock()
}

func (h *staleHold[V]) clear() {
	h.mu.Lock()
	h.m = map[string]V{}
	h.mu.Unlock()
}
