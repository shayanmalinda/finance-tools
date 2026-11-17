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

package textnorm

import "testing"

func TestLatin1(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Unchanged.
		{"empty", "", ""},
		{"plain ASCII", "Choreo Workshop - Q4", "Choreo Workshop - Q4"},
		{"line breaks and tabs", "line one\nline two\r\n\tindented", "line one\nline two\r\n\tindented"},
		{"latin1 accents", "Café São Paulo, Zürich, Ærøskøbing", "Café São Paulo, Zürich, Ærøskøbing"},
		{"latin1 symbols", "£100 ±5% ½ © ® ° × ÷ ¿¡", "£100 ±5% ½ © ® ° × ÷ ¿¡"},
		{"cp1252 extras kept", "€50 … • ™ Œuvre", "€50 … • ™ Œuvre"},
		{"curly quotes kept", "Customer’s ‘Go-Live’ “Phase 2” ‚low‘ „low“ ‹a›", "Customer’s ‘Go-Live’ “Phase 2” ‚low‘ „low“ ‹a›"},
		{"en and em dash kept", "Jan–Mar — Day 1", "Jan–Mar — Day 1"},
		{"no-break space kept", "10\u00A0km", "10\u00A0km"},

		// Replaced (LEGACY-ISSUES.md #1).
		{"right arrow", "APIM 3.2 → v4.2 Migration", "APIM 3.2 -> v4.2 Migration"},
		{"other arrows", "A ← B ↔ C ⇒ D", "A <- B <-> C => D"},
		{"quotes and primes latin1 lacks", "‛a‟ 5′ 6″", "'a\" 5' 6\""},
		{"dashes latin1 lacks", "a‒b―c", "a-b-c"},
		{"minus sign", "−5", "-5"},
		{"non-breaking hyphen", "e\u2011mail", "e-mail"},
		{"thin and narrow spaces", "10\u2009000\u202Fkm", "10 000 km"},
		{"line separator", "a\u2028b", "a\nb"},
		{"comparison signs", "≤ 5 ≥ 1 ≠ 0", "<= 5 >= 1 != 0"},
		{"letters without decomposition", "Łukasz Kołodziej", "Lukasz Kolodziej"},
		{"ligature", "ﬁnance", "finance"},

		// Decomposed to the base letter.
		{"hungarian double acute", "Erdős", "Erdos"},
		{"czech caron", "Čapek Dvořák", "Capek Dvorák"},
		{"combining accent composes to latin1", "Cafe\u0301", "Café"},
		{"combining mark with no latin1 form", "n\u0331", "n"},

		// Dropped.
		{"emoji", "Launch 🚀 Party 🎉", "Launch Party"},
		{"emoji with variation selector and ZWJ", "Team \U0001F469\u200D\U0001F4BB✔\uFE0F", "Team"},
		{"emoji mid-text keeps line breaks", "emoji 😀 and\nmore", "emoji and\nmore"},
		{"zero-width space and BOM", "a\u200Bb\uFEFFc", "abc"},
		{"CJK", "Tokyo 東京", "Tokyo"},
		{"C1 control", "a\u0085b", "ab"},
		{"invalid UTF-8", "a\xffb", "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Latin1(tc.in); got != tc.want {
				t.Errorf("Latin1(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Every output character fits a latin1 column, and normalising twice changes
// nothing.
func TestLatin1OutputFitsAndIsIdempotent(t *testing.T) {
	inputs := []string{
		"APIM 3.2 → v4.2 “Migration” – 🚀",
		"Łódź Erdős Čapek 東京 ﬁ \u2009",
		"Cafe\u0301 \u00A0 € … a\xffb \u0085",
	}
	for _, in := range inputs {
		once := Latin1(in)
		for _, r := range once {
			if !fits(r) {
				t.Errorf("Latin1(%q) = %q contains %U, which does not fit latin1", in, once, r)
			}
		}
		if twice := Latin1(once); twice != once {
			t.Errorf("not idempotent: Latin1(%q) = %q, Latin1 of that = %q", in, once, twice)
		}
	}
}

func TestCountry(t *testing.T) {
	if got := Country("\nUnited States of America "); got != "United States of America" {
		t.Errorf("Country = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{{"abc", 3, "abc"}, {"abcd", 3, "abc"}, {"é→x", 2, "é→"}, {"", 0, ""}} {
		if got := Truncate(c.in, c.n); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
