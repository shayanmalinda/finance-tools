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

// Package server assembles the HTTP server, router, and middleware chain.
package server

import (
	"net/http"
	"time"
)

const (
	serverReadHeaderTimeout = 10 * time.Second // Slowloris protection
	serverReadTimeout       = 60 * time.Second // passport and visa uploads
	serverWriteTimeout      = 60 * time.Second
	serverIdleTimeout       = 60 * time.Second
)

// New creates an http.Server with production-safe timeouts around handler
// (normally NewRouter's).
func New(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
}
