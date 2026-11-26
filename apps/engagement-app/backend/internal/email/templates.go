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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package email

import (
	"embed"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// The email templates, ported from the Email Alert Service
// (digiops-infra operations/email-alert-service/template.bal): base.html is
// its baseTemplate with the Choreo Labs title and copyright, and each other
// file is one template body, named by its template id. The backend renders
// them itself and sends the finished HTML to the Email Service.
//
//go:embed templates/*.html
var templateFiles embed.FS

//go:embed assets/logo-light.png
var logoLight []byte

//go:embed assets/logo-navy.png
var logoNavy []byte

// Inline logo content IDs, referenced from base.html as cid:<id>. The logos
// travel inside the email, so no mail client or image proxy has to fetch them.
const (
	logoLightCID = "choreo-labs-logo-light"
	logoNavyCID  = "choreo-labs-logo-navy"
)

// attachment is one Email Service attachment. Attachment is a JSON array of
// byte values, the shape the service's Ballerina byte[] expects.
type attachment struct {
	ContentName string `json:"contentName"`
	ContentType string `json:"contentType"`
	Attachment  []int  `json:"attachment"`
	ContentID   string `json:"contentId"`
}

// logoAttachments are the inline logos every email carries.
func logoAttachments() []attachment {
	return []attachment{inlinePNG(logoLightCID, logoLight), inlinePNG(logoNavyCID, logoNavy)}
}

func inlinePNG(cid string, b []byte) attachment {
	ints := make([]int, len(b))
	for i, v := range b {
		ints[i] = int(v)
	}
	return attachment{ContentName: cid + ".png", ContentType: "image/png", Attachment: ints, ContentID: cid}
}

// bodyMarker is where the base template takes the body.
const bodyMarker = "<!-- [EMAIL_BODY] -->"

// ErrUnknownTemplate means no template has that id: the email is not sent
// rather than sent blank.
var ErrUnknownTemplate = errors.New("email: unknown template")

var templateIDPattern = regexp.MustCompile(`^[A-Za-z]+$`)

// Render builds the full HTML of one email the way the Email Alert Service
// did: the template body goes into the base at <!-- [EMAIL_BODY] -->, then
// each content key replaces <!-- [KEY] --> (key upper-cased, so keys are
// case-insensitive). Values are inserted as they are: callers HTML-escape
// user text. A placeholder with no value stays an HTML comment. banner, when
// set (staging), is shown as a notice above the body.
func Render(templateID string, content map[string]string, banner string) (string, error) {
	if !templateIDPattern.MatchString(templateID) || templateID == "base" {
		return "", fmt.Errorf("%w: %q", ErrUnknownTemplate, templateID)
	}
	body, err := templateFiles.ReadFile("templates/" + templateID + ".html")
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrUnknownTemplate, templateID)
	}
	base, err := templateFiles.ReadFile("templates/base.html")
	if err != nil {
		return "", err
	}
	// The shared top of every email: greeting and what happened.
	get := func(k string) string {
		for ck, v := range content {
			if strings.EqualFold(ck, k) {
				return v
			}
		}
		return ""
	}
	top := greetingHTML(get("greeting_name"))
	if d := get("description"); d != "" {
		top += fmt.Sprintf(`<p style="margin: 0 0 16px; %s">%s</p>`+"\n", layText, d)
	}
	// The one button comes after the details, so people read before acting.
	button := ""
	if link := get("action_link"); link != "" {
		button = buttonHTML(get("action_label"), link)
	}
	reason := ""
	if r := get("reason"); r != "" {
		reason = fmt.Sprintf(`<p style="margin: 16px 0 0; %s font-size: 13px; line-height: 20px; color: %s;">%s</p>`, layFont, colSlate, html.EscapeString(r))
	}
	merged := map[string]string{"reason_html": reason}
	for k, v := range content {
		merged[k] = v
	}
	content = merged
	page := strings.Replace(string(base), bodyMarker, bannerRow(banner)+top+string(body)+button, 1)
	// Literal replacement in a fixed order (the service used a regex
	// replace, where a "$" in a value is special, in map order).
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		page = strings.ReplaceAll(page, "<!-- ["+strings.ToUpper(k)+"] -->", content[k])
	}
	return asciiHTML(page), nil
}

// asciiHTML writes every non-ASCII character as a numeric HTML entity. The
// Email Service decodes the base64 body without UTF-8, so a raw "–" or "é"
// arrives as "?"; entities survive any decoding.
func asciiHTML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 128 {
			b.WriteRune(r)
			continue
		}
		fmt.Fprintf(&b, "&#%d;", r)
	}
	return b.String()
}

// bannerRow is the staging notice above the body; empty when banner is.
func bannerRow(banner string) string {
	if banner == "" {
		return ""
	}
	return `<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width: 100%; margin: 0 0 24px;"><tr>` +
		`<td bgcolor="#F4F6FA" style="background-color: #F4F6FA; border-left: 4px solid #FF4600; border-radius: 6px; padding: 12px 16px; ` +
		`font-family: 'Inter', 'Segoe UI', Helvetica, Arial, sans-serif; font-size: 14px; line-height: 20px; color: #0B1633;">` +
		banner + `</td></tr></table>`
}
