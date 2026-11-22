// Package lexical is the keyword matching `orai search` uses where no index answers:
// the mailbox (never indexed, so a role's mail stays out of the shared document index)
// and the documents when the shelf server cannot be reached. It is deliberately
// simple: case-insensitive substring counts over query terms, with a crude stem for
// Korean words so a particle on either side ("신원을" against "신원이") still matches.
package lexical

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Term is one query word and the shorter forms that also count as a match.
type Term struct {
	Word  string
	Forms []string
}

// Terms splits a query into lower-cased words of at least two characters, without
// duplicates, in query order.
func Terms(query string) []Term {
	seen := map[string]bool{}
	var terms []Term
	for _, field := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-' && r != '.'
	}) {
		field = strings.Trim(field, "-.")
		if utf8.RuneCountInString(field) < 2 || seen[field] {
			continue
		}
		seen[field] = true
		forms := []string{field}
		if stem := hangulStem(field); stem != "" {
			forms = append(forms, stem)
		}
		terms = append(terms, Term{Word: field, Forms: forms})
	}
	return terms
}

// hangulStem drops the last syllable of a Hangul word of three or more syllables,
// which is usually a particle or ending ("신원을" → "신원"). Empty when not applicable.
func hangulStem(word string) string {
	runes := []rune(word)
	if len(runes) < 3 || !unicode.Is(unicode.Hangul, runes[len(runes)-1]) {
		return ""
	}
	return string(runes[:len(runes)-1])
}

// Match is how one text answers a query.
type Match struct {
	Distinct int    // query terms found at least once
	Total    int    // occurrences of all terms
	Line     int    // 1-based line with the most occurrences (0 when none)
	Excerpt  string // that line, trimmed
}

// Enough reports whether a match is worth showing: a one- or two-term query must find
// every term, a longer one at least half of them (a natural-language question carries
// words a document need not repeat).
func (m Match) Enough(terms []Term) bool {
	if len(terms) == 0 || m.Distinct == 0 {
		return false
	}
	if len(terms) <= 2 {
		return m.Distinct == len(terms)
	}
	return m.Distinct*2 >= len(terms)
}

// Score matches text against terms line by line.
func Score(text string, terms []Term) Match {
	var m Match
	found := make([]bool, len(terms))
	best := 0
	for i, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		count := 0
		for t, term := range terms {
			n := occurrences(lower, term)
			if n > 0 {
				found[t] = true
				count += n
			}
		}
		m.Total += count
		if count > best {
			best, m.Line, m.Excerpt = count, i+1, Trim(line, 200)
		}
	}
	for _, f := range found {
		if f {
			m.Distinct++
		}
	}
	return m
}

// occurrences counts a term in a lower-cased line, through its longest matching form.
func occurrences(lower string, term Term) int {
	for _, form := range term.Forms {
		if n := strings.Count(lower, form); n > 0 {
			return n
		}
	}
	return 0
}

// Trim shortens s to at most n characters (not bytes), marking the cut.
func Trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// Rank orders matches best first: more distinct terms, then more occurrences. Ties
// keep the order the caller supplied (for example newest first).
func Rank[T any](items []T, match func(T) Match) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := match(items[i]), match(items[j])
		if a.Distinct != b.Distinct {
			return a.Distinct > b.Distinct
		}
		return a.Total > b.Total
	})
}
