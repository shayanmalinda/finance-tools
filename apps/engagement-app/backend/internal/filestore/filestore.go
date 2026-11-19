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

// Package filestore keeps visa and travel document images in the legacy
// Azure Blob containers; a key is "<container>/<blob name>".
package filestore

import (
	"context"
	"errors"
	"io"
)

// Object describes a stored file.
type Object struct {
	Key         string
	ContentType string
	Size        int64
}

// Store keeps file bytes.
type Store interface {
	// Put stores size bytes from r under key.
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
	// Get opens the file at key. The caller closes the reader.
	Get(ctx context.Context, key string) (io.ReadCloser, Object, error)
	// Delete removes the file at key; a missing file is not an error.
	Delete(ctx context.Context, key string) error
}

// ErrNotConfigured is returned by Unconfigured: uploads and downloads answer
// 503 when file storage is not set up (local runs).
var ErrNotConfigured = errors.New("file storage is not configured")

// ErrNotFound is returned by Get for a missing blob.
var ErrNotFound = errors.New("file not found")

// Unconfigured is the Store used when FILE_STORAGE_ACCOUNT is unset.
type Unconfigured struct{}

// Put implements Store.
func (Unconfigured) Put(context.Context, string, string, io.Reader, int64) error {
	return ErrNotConfigured
}

// Get implements Store.
func (Unconfigured) Get(context.Context, string) (io.ReadCloser, Object, error) {
	return nil, Object{}, ErrNotConfigured
}

// Delete implements Store.
func (Unconfigured) Delete(context.Context, string) error {
	return ErrNotConfigured
}
