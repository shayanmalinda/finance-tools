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

// Package textnorm makes free text fit the legacy latin1 columns.
//
// The engagement (and other legacy) text columns are MySQL latin1, which is
// Windows-1252. In strict mode MySQL rejects an INSERT or UPDATE carrying a
// character the column cannot hold ("Incorrect string value: '\xE2\x86\x92'
// for column 'project_name'", LEGACY-ISSUES.md #1), so the whole save fails.
// Latin1 normalises a string before it is written so that a save never fails
// on a character.
package textnorm

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// replacements maps characters a latin1 column cannot hold to their closest
// plain equivalent: arrows, the quotes, dashes and spaces Windows-1252 lacks,
// comparison signs, and letters with no decomposition. Characters
// Windows-1252 does hold (curly quotes, en and em dash, ellipsis) are never
// listed here: they are stored as they are.
//
// Invisible characters are written as escapes so they can be reviewed.
var replacements = map[rune]string{
	// Arrows.
	'→': "->", '←': "<-", '↔': "<->", '⇒': "=>", '⇐': "<=", '⇔': "<=>",
	'⟶': "->", '⟵': "<-", '➔': "->", '➜': "->", '➝': "->", '➞': "->",
	// Quotes and primes Windows-1252 lacks.
	'‛': "'", '′': "'", '‟': `"`, '″': `"`,
	// Hyphens, dashes and minus signs Windows-1252 lacks.
	'\u2010': "-", '\u2011': "-", '‒': "-", '―': "-", '−': "-",
	// Typographic spaces. U+00A0 (no-break space) fits latin1 and is kept.
	'\u2002': " ", '\u2003': " ", '\u2004': " ", '\u2005': " ", '\u2006': " ",
	'\u2007': " ", '\u2008': " ", '\u2009': " ", '\u200A': " ", '\u202F': " ",
	'\u205F': " ", '\u3000': " ",
	// Line and paragraph separators.
	'\u2028': "\n", '\u2029': "\n",
	// Comparison signs.
	'≤': "<=", '≥': ">=", '≠': "!=",
	// Letters with no decomposition but an obvious plain form. ø and Ø fit
	// latin1 and are kept, so they are not listed.
	'ł': "l", 'Ł': "L", 'đ': "d", 'Đ': "D", 'ı': "i",
	'ħ': "h", 'Ħ': "H", 'ŋ': "n", 'Ŋ': "N", 'ŧ': "t", 'Ŧ': "T",
	// Ligatures.
	'ﬀ': "ff", 'ﬁ': "fi", 'ﬂ': "fl", 'ﬃ': "ffi", 'ﬄ': "ffl",
}

// cp1252Extras are the characters Windows-1252 stores in 0x80-0x9F, beyond
// ISO-8859-1. MySQL latin1 accepts them, so they are kept as they are.
var cp1252Extras = map[rune]bool{
	'€': true, '‚': true, 'ƒ': true, '„': true, '…': true, '†': true, '‡': true,
	'ˆ': true, '‰': true, 'Š': true, '‹': true, 'Œ': true, 'Ž': true, '‘': true,
	'’': true, '“': true, '”': true, '•': true, '–': true, '—': true, '˜': true,
	'™': true, 'š': true, '›': true, 'œ': true, 'ž': true, 'Ÿ': true,
}

// fits reports whether a MySQL latin1 column holds r as itself. C1 control
// characters (U+0080-U+009F) do not count: they are never meaningful text.
func fits(r rune) bool {
	switch {
	case r < 0x80:
		return true
	case r >= 0xA0 && r <= 0xFF:
		return true
	default:
		return cp1252Extras[r]
	}
}

// Latin1 returns s with every character a MySQL latin1 column cannot hold
// replaced by its closest plain equivalent, or dropped when there is none.
//
// In order, for each character of s (after composing it to NFC, so "e" plus a
// combining accent becomes "é", which fits):
//
//  1. A character that fits is kept ("é", "ü", "€", "’", "–", a line break).
//  2. A character in replacements becomes its replacement ("→" becomes "->").
//  3. A character whose decomposition starts with a letter that fits keeps
//     that letter and loses its marks ("ő" becomes "o", "Č" becomes "C").
//  4. Anything else is dropped: emoji, pictographs, CJK, zero-width and
//     other invisible characters, C1 controls, invalid UTF-8.
//
// Latin1 is pure and idempotent.
func Latin1(s string) string {
	if isASCII(s) {
		return s
	}
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	dropped := false
	for _, r := range s {
		if fits(r) {
			b.WriteRune(r)
			continue
		}
		if rep, ok := replacements[r]; ok {
			b.WriteString(rep)
			continue
		}
		// unicode.ReplacementChar (what ranging over invalid UTF-8 yields)
		// and every other unmatched character fall through to baseLetter,
		// which drops them.
		if base, ok := baseLetter(r); ok {
			b.WriteRune(base)
		} else {
			dropped = true
		}
	}
	if !dropped {
		return b.String()
	}
	// A dropped character (an emoji) leaves its spaces behind: collapse them.
	out := b.String()
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return strings.TrimSpace(out)
}

// baseLetter returns the first rune of r's canonical decomposition when that
// rune is a letter that fits, e.g. 'ő' gives 'o'.
// Country is Latin1 trimmed (legacy rows carry stray line breaks).
func Country(s string) string { return strings.TrimSpace(Latin1(s)) }

func baseLetter(r rune) (rune, bool) {
	for _, c := range norm.NFD.String(string(r)) {
		if unicode.IsLetter(c) && fits(c) {
			return c, true
		}
		break
	}
	return 0, false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Truncate cuts s to at most n characters.
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
