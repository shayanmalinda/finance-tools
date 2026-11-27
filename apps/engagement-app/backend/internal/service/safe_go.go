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
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
)

// goSafe runs fn in a goroutine on wg. A panic is logged and, when errp is
// set, stored there, so it fails the request instead of ending the process.
func goSafe(ctx context.Context, wg *sync.WaitGroup, errp *error, fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "service: goroutine panic", "panic", r, "stack", string(debug.Stack()))
				if errp != nil {
					*errp = fmt.Errorf("panic: %v", r)
				}
			}
		}()
		fn()
	}()
}
