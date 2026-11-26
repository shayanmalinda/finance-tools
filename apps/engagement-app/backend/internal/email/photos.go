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

package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Profile photos in the details block. A builder marks a person row with the
// person's address (personMarker); when the email is sent, every marker whose
// photo is known becomes an inline image and the rest are removed. A photo is
// never linked: Gmail's image proxy cannot fetch protected hosts, so the
// bytes travel inside the email like the logos. A photo that cannot be found
// or fetched is left out, and the email is sent without it.

const (
	// maxPhotoBytes is the largest photo attached; a bigger one is skipped.
	maxPhotoBytes = 200 << 10
	// photoBudget bounds all the photo work of one email.
	photoBudget = 8 * time.Second
	// photoFetchTimeout bounds one image download.
	photoFetchTimeout = 4 * time.Second
	photoCacheMax     = 500
	photoHitTTL       = 6 * time.Hour
	photoMissTTL      = 10 * time.Minute
	photoSize         = 32
)

// Photo is a profile photo ready to attach.
type Photo struct {
	Bytes       []byte
	ContentType string
}

// PhotoSource finds the photo of the person with this address.
type PhotoSource interface {
	Photo(ctx context.Context, addr string) (Photo, bool)
}

var photoMarkerRe = regexp.MustCompile(`<!-- \[PHOTO:([0-9a-f]+)\] -->`)

// personMarker is the placeholder a person row carries in front of the name;
// the address is hex-encoded so it can never break out of the comment.
func personMarker(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return ""
	}
	return "<!-- [PHOTO:" + hex.EncodeToString([]byte(addr)) + "] -->"
}

// photoCID is the content ID of a person's inline photo.
func photoCID(addr string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(addr))))
	return "person-" + hex.EncodeToString(sum[:8])
}

// photoImg is the round photo shown before a name; a client that ignores the
// border radius shows it square.
func photoImg(cid string) string {
	return fmt.Sprintf(`<img src="cid:%s" width="%d" height="%d" alt="" style="display: inline-block; width: %dpx; height: %dpx; `+
		`border: 0; border-radius: 50%%; vertical-align: middle; margin: 0 10px 0 0;">`, html.EscapeString(cid), photoSize, photoSize, photoSize, photoSize)
}

func photoExt(contentType string) string {
	switch contentType {
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	}
	return "jpg"
}

// inlinePhotos replaces the person markers in page: a known photo becomes an
// inline image (and an attachment, once per person), any other marker is
// removed. It never fails; the attachments are only the photos the page uses.
func inlinePhotos(ctx context.Context, src PhotoSource, page string) (string, []attachment) {
	if !photoMarkerRe.MatchString(page) {
		return page, nil
	}
	if src != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, photoBudget)
		defer cancel()
	}
	found := map[string]string{} // hex address -> cid ("" when no photo)
	var atts []attachment
	page = photoMarkerRe.ReplaceAllStringFunc(page, func(marker string) string {
		h := photoMarkerRe.FindStringSubmatch(marker)[1]
		cid, done := found[h]
		if !done {
			if raw, err := hex.DecodeString(h); err == nil && src != nil {
				if p, ok := src.Photo(ctx, string(raw)); ok && len(p.Bytes) > 0 && len(p.Bytes) <= maxPhotoBytes {
					cid = photoCID(string(raw))
					atts = append(atts, inlineImage(cid, p))
				}
			}
			found[h] = cid
		}
		if cid == "" {
			return ""
		}
		return photoImg(cid)
	})
	return page, atts
}

func inlineImage(cid string, p Photo) attachment {
	ints := make([]int, len(p.Bytes))
	for i, v := range p.Bytes {
		ints[i] = int(v)
	}
	return attachment{ContentName: cid + "." + photoExt(p.ContentType), ContentType: p.ContentType, Attachment: ints, ContentID: cid}
}

// ThumbnailLookup returns the thumbnail HR holds for an address: a data URI
// or an https URL ("" when there is none).
type ThumbnailLookup func(ctx context.Context, addr string) (string, error)

// PhotoFetcher is the PhotoSource that reads the HR thumbnail and downloads
// it, with a small cache so a busy hour does not repeat the lookups.
type PhotoFetcher struct {
	lookup ThumbnailLookup
	http   *http.Client
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]photoEntry
}

type photoEntry struct {
	photo   Photo
	ok      bool
	expires time.Time
}

// NewPhotoFetcher returns a PhotoFetcher over lookup. Downloads refuse
// anything but https and any non-public address.
func NewPhotoFetcher(lookup ThumbnailLookup) *PhotoFetcher {
	dialer := &net.Dialer{Timeout: photoFetchTimeout, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
			return errors.New("photo host is not a public address")
		}
		return nil
	}}
	client := &http.Client{
		Timeout:   photoFetchTimeout,
		Transport: &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: photoFetchTimeout},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Scheme != "https" {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	return &PhotoFetcher{lookup: lookup, http: client, now: time.Now, cache: map[string]photoEntry{}}
}

// Photo implements PhotoSource.
func (f *PhotoFetcher) Photo(ctx context.Context, addr string) (Photo, bool) {
	key := strings.ToLower(strings.TrimSpace(addr))
	f.mu.Lock()
	e, hit := f.cache[key]
	f.mu.Unlock()
	if hit && f.now().Before(e.expires) {
		return e.photo, e.ok
	}
	p, err := f.load(ctx, key)
	e = photoEntry{photo: p, ok: err == nil, expires: f.now().Add(photoHitTTL)}
	if err != nil {
		e.expires = f.now().Add(photoMissTTL)
	}
	f.mu.Lock()
	if len(f.cache) >= photoCacheMax {
		f.cache = map[string]photoEntry{}
	}
	f.cache[key] = e
	f.mu.Unlock()
	return p, err == nil
}

func (f *PhotoFetcher) load(ctx context.Context, addr string) (Photo, error) {
	thumb, err := f.lookup(ctx, addr)
	if err != nil {
		return Photo{}, err
	}
	thumb = strings.TrimSpace(thumb)
	switch {
	case thumb == "":
		return Photo{}, errors.New("no thumbnail")
	case strings.HasPrefix(thumb, "data:"):
		_, b64, ok := strings.Cut(thumb, ";base64,")
		if !ok || len(b64) > maxPhotoBytes*2 {
			return Photo{}, errors.New("unsupported data URI")
		}
		b, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return Photo{}, err
		}
		return checkPhoto(b)
	case strings.HasPrefix(thumb, "https://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumb, nil)
		if err != nil {
			return Photo{}, err
		}
		resp, err := f.http.Do(req)
		if err != nil {
			return Photo{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return Photo{}, fmt.Errorf("photo status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes+1))
		if err != nil {
			return Photo{}, err
		}
		return checkPhoto(b)
	}
	return Photo{}, errors.New("unsupported thumbnail")
}

// checkPhoto accepts a PNG, JPEG or GIF within the size cap (the type comes
// from the bytes, not from what the host claims).
func checkPhoto(b []byte) (Photo, error) {
	if len(b) == 0 || len(b) > maxPhotoBytes {
		return Photo{}, errors.New("photo is empty or too large")
	}
	ct := http.DetectContentType(bytes.Clone(b))
	switch ct {
	case "image/png", "image/jpeg", "image/gif":
		return Photo{Bytes: b, ContentType: ct}, nil
	}
	return Photo{}, errors.New("photo is not a png, jpeg or gif")
}
