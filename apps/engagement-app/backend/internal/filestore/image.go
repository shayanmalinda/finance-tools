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

package filestore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Containers of the legacy Travel app.
const (
	VisasContainer           = "visas"
	TravelDocumentsContainer = "travel-documents"
)

// MaxImageBytes is the upload limit (LEGACY-ISSUES C4: 5 MB).
const MaxImageBytes = 5 << 20

// ErrUnsupportedImage is returned for a file that is not a JPEG or PNG by
// its content.
var ErrUnsupportedImage = errors.New("the file must be a JPEG or PNG image")

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}_.\-@]`)

// BlobName builds the legacy blob name "{ownerEmail}_{uuid}_{imageName}"; the
// legacy UI's imageName is "{countryCode}.{extension}".
func BlobName(ownerEmail, countryCode, ext string) string {
	img := unsafeName.ReplaceAllString(countryCode, "") + "." + ext
	return ownerEmail + "_" + uuid.NewString() + "_" + img
}

// CheckImage sniffs data and returns its content type and file extension,
// or ErrUnsupportedImage.
func CheckImage(data []byte) (contentType, ext string, err error) {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return "image/jpeg", "jpg", nil
	case "image/png":
		return "image/png", "png", nil
	}
	return "", "", ErrUnsupportedImage
}

// EncodeLegacy returns the blob content the legacy Travel app writes and
// reads: the image as a data URL text ("data:image/png;base64,...").
func EncodeLegacy(contentType string, data []byte) []byte {
	return []byte("data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data))
}

// DecodeLegacy turns stored blob content back into image bytes. Legacy blobs
// hold a data URL text; anything else is returned as is with its sniffed type.
func DecodeLegacy(blob []byte) (contentType string, data []byte, err error) {
	s := string(bytes.TrimSpace(blob))
	if strings.HasPrefix(s, "data:") {
		meta, payload, ok := strings.Cut(s, ",")
		if !ok || !strings.HasSuffix(meta, ";base64") {
			return "", nil, fmt.Errorf("filestore: malformed data URL")
		}
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, fmt.Errorf("filestore: data URL payload: %w", err)
		}
		return strings.TrimSuffix(strings.TrimPrefix(meta, "data:"), ";base64"), data, nil
	}
	return http.DetectContentType(blob), blob, nil
}
