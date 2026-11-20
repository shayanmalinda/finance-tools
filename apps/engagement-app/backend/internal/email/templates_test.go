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
	"errors"
	"strings"
	"testing"
)

// Every template id the backend sends renders a complete Choreo Labs email.
func TestRender_EveryTemplate(t *testing.T) {
	ids := []string{
		"customerEngagement", "internalEngagement", "marketingEvent",
		"customerEngagementCancelled", "internalEngagementCancelled", "marketingEventCancelled",
		"customerEngagementAllocation", "internalEngagementAllocation", "marketingEventAllocation",
		"consultantConfirmed", TravelTemplateAssign, TravelTemplateCompleted, TravelTemplateNoContent,
	}
	content := map[string]string{"CURRENT_YEAR": "2026", "CLOSING_AND_SIGNATURE": digitalTeamSignature, "description": "Body text"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			page, err := Render(id, content, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"<title>Choreo Labs</title>", "© 2026 Choreo Labs. All Rights Reserved", "Choreo Labs Digital Team", "Body text",
				"choreo-labs-logo-light-400px.png", "choreo-labs-logo-navy-400px.png", `href="https://choreolabs.com/"`, `alt="Choreo Labs Logo"`,
				// Responsive: a viewport, a 600px card that goes full width on small screens.
				`<meta name="viewport" content="width=device-width, initial-scale=1">`, "max-width: 600px;", "@media only screen and (max-width: 620px)"} {
				if !strings.Contains(page, want) {
					t.Errorf("lacks %q", want)
				}
			}
			for _, gone := range []string{bodyMarker, "WSO2, Inc. All Rights Reserved", "<title>WSO2", "WSO2-Logo", `alt="WSO2 Logo"`, `href="https://wso2.com/"`, "twitter-dark", "Roboto"} {
				if strings.Contains(page, gone) {
					t.Errorf("still has %q", gone)
				}
			}
		})
	}
}

// Keys are case-insensitive and inserted literally ("$" is not special);
// a placeholder with no value stays a comment; the banner shows only when set.
func TestRender_Placeholders(t *testing.T) {
	page, err := Render("noContent", map[string]string{"Description": "Pay $1 &amp; go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "Pay $1 &amp; go") || !strings.Contains(page, "<!-- [TRAVEL_AGENT_NAME] -->") {
		t.Error("placeholder replacement is wrong")
	}
	if strings.Contains(page, "fff4e5") {
		t.Error("banner shown without a banner")
	}
	if page, _ = Render("noContent", nil, "Staging notice"); !strings.Contains(page, "Staging notice") || !strings.Contains(page, "fff4e5") {
		t.Error("banner missing")
	}
}

func TestRender_UnknownTemplate(t *testing.T) {
	for _, id := range []string{"", "nope", "base", "../base", "noContent.html"} {
		if _, err := Render(id, nil, ""); !errors.Is(err, ErrUnknownTemplate) {
			t.Errorf("%q: err = %v", id, err)
		}
	}
}
