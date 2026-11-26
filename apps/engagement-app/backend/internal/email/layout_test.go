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
	"strings"
	"testing"
)

// The person block: the name, and under it the address in a smaller Slate
// line; the address alone when the name is unknown. Everything is escaped.
func TestPersonRow_Block(t *testing.T) {
	both := detailsSection("Trip", personRow("Traveller", "Shayan <Malinda>", "shayan@wso2.com"))
	for _, want := range []string{"Shayan &lt;Malinda&gt;<br>", "shayan@wso2.com</span>", colSlate} {
		if !strings.Contains(both, want) {
			t.Errorf("block lacks %q: %s", want, both)
		}
	}
	only := detailsSection("Trip", personRow("Requested By", "", "req@wso2.com"))
	if strings.Count(only, "req@wso2.com") != 1 || strings.Contains(only, "<br>") {
		t.Errorf("address-only block = %s", only)
	}
	same := detailsSection("Trip", personRow("Consultant", "jane@wso2.com", "jane@wso2.com"))
	if strings.Count(same, "jane@wso2.com") != 1 {
		t.Errorf("a name equal to the address is shown once: %s", same)
	}
	if detailsSection("Trip", personRow("Traveller", "", "")) != "" {
		t.Error("a person with no name and no address should be left out")
	}
}

// Every value is HTML-escaped: text that looks like markup stays text.
func TestDetailsSection_EscapesValuesAndCentresRows(t *testing.T) {
	evil := `<img src="cid:evil"><!-- [PHOTO:6a:J] -->`
	b := detailsSection("Trip", row{"Job Number", evil}, personRow("Traveller", evil, evil))
	if strings.Contains(b, "<img") || strings.Contains(b, "<!-- [PHOTO") || !strings.Contains(b, "&lt;img") {
		t.Errorf("value was not escaped: %s", b)
	}
	if strings.Count(b, `valign="middle"`) != 4 || strings.Contains(b, "vertical-align: top") {
		t.Errorf("label and value cells should be vertically centred: %s", b)
	}
}
