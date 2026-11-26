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
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type fakePhotos map[string]Photo

func (f fakePhotos) Photo(_ context.Context, addr string) (Photo, bool) {
	p, ok := f[strings.ToLower(addr)]
	return p, ok
}

func photoSender(t *testing.T, src PhotoSource) (*HTTPSender, *payload, func()) {
	t.Helper()
	var got payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewDecoder(r.Body).Decode(&got) }))
	s := New(Config{ServiceURL: srv.URL}, rest.New("email", srv.URL, srv.Client())).WithPhotos(src)
	return s, &got, srv.Close
}

func travellerMessage() Message {
	trip := TravelTrip{JobNumber: "TRV-1", TravellerEmail: "Jane@wso2.com", TravellerName: "Jane Doe", Country: "Sri Lanka"}
	m := TravelNotice(TravelSettings{LogisticsGroup: "l@wso2.com", AppBaseURL: "https://one.wso2.com"}, trip, "Travel On Hold", "x", "")
	return m
}

// A person with a photo gets a round inline image before the name, and the
// photo is attached once; the email carries no link to the photo.
func TestPhotos_ShownAndAttached(t *testing.T) {
	s, got, done := photoSender(t, fakePhotos{"jane@wso2.com": {Bytes: testPNG(t), ContentType: "image/png"}})
	defer done()
	if err := s.Send(context.Background(), travellerMessage()); err != nil {
		t.Fatal(err)
	}
	page := decoded(t, *got)
	cid := photoCID("jane@wso2.com")
	if !strings.Contains(page, `src="cid:`+cid+`"`) || !strings.Contains(page, "border-radius: 50%") || !strings.Contains(page, `alt=""`) ||
		!strings.Contains(page, `width="32" height="32"`) || strings.Contains(page, "[PHOTO:") {
		t.Errorf("page lacks the photo: %s", page)
	}
	if strings.Index(page, "cid:"+cid) > strings.Index(page, ">Jane Doe<") && strings.Contains(page, ">Jane Doe<") {
		t.Error("photo is not before the name")
	}
	var photos []attachment
	for _, a := range got.Attachments {
		if a.ContentID == cid {
			photos = append(photos, a)
		}
	}
	if len(photos) != 1 || photos[0].ContentType != "image/png" || len(photos[0].Attachment) != len(testPNG(t)) || len(got.Attachments) != 3 {
		t.Errorf("attachments = %d, photo = %+v", len(got.Attachments), photos)
	}
}

// Without a photo the row is unchanged and only the logos are attached.
func TestPhotos_NoPhotoNoAttachment(t *testing.T) {
	for name, src := range map[string]PhotoSource{"empty": fakePhotos{}, "none": nil} {
		s, got, done := photoSender(t, src)
		if err := s.Send(context.Background(), travellerMessage()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		page := decoded(t, *got)
		if strings.Contains(page, "cid:person-") || strings.Contains(page, "[PHOTO:") || !strings.Contains(page, ">Jane Doe<") || len(got.Attachments) != 2 {
			t.Errorf("%s: page or attachments wrong (%d attachments)", name, len(got.Attachments))
		}
		done()
	}
}

// A photo that is too large is skipped, and an email naming nobody never asks
// the source (so only photos the email references are attached).
func TestPhotos_OversizeSkippedAndOnlyReferenced(t *testing.T) {
	s, got, done := photoSender(t, fakePhotos{"jane@wso2.com": {Bytes: make([]byte, maxPhotoBytes+1), ContentType: "image/png"},
		"other@wso2.com": {Bytes: testPNG(t), ContentType: "image/png"}})
	defer done()
	if err := s.Send(context.Background(), travellerMessage()); err != nil {
		t.Fatal(err)
	}
	if len(got.Attachments) != 2 || strings.Contains(decoded(t, *got), "cid:person-") {
		t.Errorf("attachments = %d", len(got.Attachments))
	}
}

// A failing photo fetch never fails the email: it goes out without the photo.
func TestPhotos_FetchFailureStillSends(t *testing.T) {
	f := NewPhotoFetcher(func(context.Context, string) (string, error) { return "", errors.New("hr down") })
	s, got, done := photoSender(t, f)
	defer done()
	if err := s.Send(context.Background(), travellerMessage()); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if page := decoded(t, *got); !strings.Contains(page, ">Jane Doe<") || strings.Contains(page, "cid:person-") || len(got.Attachments) != 2 {
		t.Errorf("email should go without the photo: %d attachments", len(got.Attachments))
	}
}

func TestPhotoFetcher_DataURIAndRules(t *testing.T) {
	png := testPNG(t)
	calls := 0
	lookup := func(_ context.Context, addr string) (string, error) {
		calls++
		switch addr {
		case "ok@wso2.com":
			return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
		case "svg@wso2.com":
			return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte("<svg/>")), nil
		case "http@wso2.com":
			return "http://example.com/a.png", nil
		case "local@wso2.com":
			return "https://127.0.0.1/a.png", nil
		}
		return "", nil
	}
	f := NewPhotoFetcher(lookup)
	if p, ok := f.Photo(context.Background(), "OK@wso2.com"); !ok || p.ContentType != "image/png" {
		t.Errorf("data URI = %+v %v", p, ok)
	}
	f.Photo(context.Background(), "ok@wso2.com")
	if calls != 1 {
		t.Errorf("lookups = %d, want the second answered from the cache", calls)
	}
	for _, a := range []string{"svg@wso2.com", "http@wso2.com", "local@wso2.com", "none@wso2.com"} {
		if _, ok := f.Photo(context.Background(), a); ok {
			t.Errorf("%s: photo should be skipped", a)
		}
	}
}
