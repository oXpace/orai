package lexical

import (
	"math"
	"path"
	"sort"
	"strings"
)

// The document ranker below is what `orai search` uses for the shelf folders when no
// engine answers. Its ideas come from wikimap 1.5.1 (MIT, github.com/dhha22/wikimap):
// rank sections rather than whole files and point at the line, weight a word by how
// rare it is in the corpus (IDF), count a hit in the title, path or heading for more
// than one in the body, judge coverage over the whole document, fuse several phrasings
// of one question by reciprocal rank, and say when a result is weak. No code was
// copied; the weights are Orai's own.

// Field weights, in units of a term's IDF: a hit in the document title or path, or in
// the heading of the section shown, says more than one in body text.
const (
	titleWeight   = 8
	pathWeight    = 6
	headingWeight = 5
	lineWeight    = 2
	// rrfK damps reciprocal rank fusion so a single first place does not outweigh
	// agreement between phrasings (the usual constant).
	rrfK = 60
	// weakCoverage is the share of a query's weight below which the best document is
	// a weak answer. A question carries words no answer repeats, so not 1.
	weakCoverage = 0.6
)

// Doc is one document to rank, split into sections.
type Doc struct {
	Path     string // slash-separated, as the caller names it
	Title    string
	Sections []Section
}

// Section is a run of lines under one heading. Line is the 1-based line of its first
// line in the file.
type Section struct {
	Heading string
	Line    int
	Lines   []string
}

// ParseMarkdown splits a document into sections at ATX headings outside fenced code.
// The title is the front matter `title:`, else the first level-1 heading, else the
// file name without extension.
func ParseMarkdown(name, text string) Doc {
	doc := Doc{Path: name}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "---" || trimmed == "..." {
				start = i + 1
				break
			}
			if value, ok := strings.CutPrefix(trimmed, "title:"); ok && doc.Title == "" {
				doc.Title = strings.Trim(strings.TrimSpace(value), `"'`)
			}
		}
	}
	current := Section{Line: start + 1}
	fence := ""
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			current.Lines = append(current.Lines, line)
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			current.Lines = append(current.Lines, line)
			continue
		}
		if level, heading := atxHeading(trimmed); level > 0 {
			if len(current.Lines) > 0 || current.Heading != "" {
				doc.Sections = append(doc.Sections, current)
			}
			if level == 1 && doc.Title == "" {
				doc.Title = heading
			}
			current = Section{Heading: heading, Line: i + 1, Lines: []string{line}}
			continue
		}
		current.Lines = append(current.Lines, line)
	}
	if len(current.Lines) > 0 || current.Heading != "" {
		doc.Sections = append(doc.Sections, current)
	}
	if doc.Title == "" {
		doc.Title = strings.TrimSuffix(path.Base(name), path.Ext(name))
	}
	return doc
}

// atxHeading returns the level and text of a Markdown heading line, or 0.
func atxHeading(line string) (int, string) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || (level < len(line) && line[level] != ' ' && line[level] != '\t') {
		return 0, ""
	}
	return level, strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line[level:]), "#"))
}

// DocHit is one ranked document: where to look and why.
type DocHit struct {
	Path    string
	Title   string
	Heading string   // of the section shown
	Line    int      // best line in that section, 1-based
	Excerpt string   // that line, trimmed
	Score   float64  // a single phrasing's score, or the fused score of several
	Missing []string // query words of the best phrasing this document does not contain
	// Coverage is the share of the query's weight this document covers, 0 to 1. A word
	// no document contains weighs as much as the rarest word could.
	Coverage float64
}

// DocResult is a ranked search over a corpus.
type DocResult struct {
	Hits []DocHit
	// Weak says the result is worth a second try with other words: nothing matched,
	// or the top document covers less than weakCoverage of the query's weight.
	Weak bool
	// Unknown lists query words that occur in no document at all (for every phrasing),
	// which no ranking can fix: the corpus says it differently.
	Unknown []string
}

// SearchDocs ranks docs for one or more phrasings of the same question, best first,
// at most limit (0 = all). With several phrasings each is ranked alone and the lists
// are fused by reciprocal rank, so a document several phrasings agree on rises.
func SearchDocs(docs []Doc, phrasings []string, limit int) DocResult {
	prepared := make([]preparedDoc, len(docs))
	for i, d := range docs {
		prepared[i] = prepare(d)
	}
	var lists [][]DocHit
	unknown := map[string]int{}
	asked := 0
	weakTop := true
	for _, q := range phrasings {
		terms := Terms(q)
		if len(terms) == 0 {
			continue
		}
		asked++
		hits, dead := rankOne(prepared, terms)
		for _, w := range dead {
			unknown[w]++
		}
		if len(hits) > 0 && hits[0].Coverage >= weakCoverage {
			weakTop = false
		}
		lists = append(lists, hits)
	}
	result := DocResult{}
	for w, n := range unknown {
		if n == asked {
			result.Unknown = append(result.Unknown, w)
		}
	}
	sort.Strings(result.Unknown)
	if len(lists) == 1 {
		result.Hits = lists[0]
	} else {
		result.Hits = fuse(lists)
	}
	if limit > 0 && len(result.Hits) > limit {
		result.Hits = result.Hits[:limit]
	}
	result.Weak = len(result.Hits) == 0 || weakTop
	return result
}

// preparedDoc caches the lower-cased fields a ranking reads.
type preparedDoc struct {
	doc      Doc
	title    string
	path     string
	headings []string
	lines    [][]string // per section, lower-cased
	all      string     // title, path and every line, for document frequency
}

func prepare(d Doc) preparedDoc {
	p := preparedDoc{doc: d, title: strings.ToLower(d.Title), path: strings.ToLower(d.Path)}
	var all strings.Builder
	all.WriteString(p.title + "\n" + p.path + "\n")
	for _, s := range d.Sections {
		p.headings = append(p.headings, strings.ToLower(s.Heading))
		lower := make([]string, len(s.Lines))
		for i, l := range s.Lines {
			lower[i] = strings.ToLower(l)
			all.WriteString(lower[i] + "\n")
		}
		p.lines = append(p.lines, lower)
	}
	p.all = all.String()
	return p
}

// rankOne ranks the corpus for one phrasing. It also returns the words no document
// contains.
func rankOne(docs []preparedDoc, terms []Term) ([]DocHit, []string) {
	n := float64(len(docs))
	idf := make([]float64, len(terms))
	var live []int
	var dead []string
	for t, term := range terms {
		df := 0
		for _, d := range docs {
			if occurrences(d.all, term) > 0 {
				df++
			}
		}
		if df == 0 {
			dead = append(dead, term.Word)
			continue
		}
		idf[t] = math.Log(1 + n/float64(df))
		live = append(live, t)
	}
	// A one- or two-word query needs every word, so one the corpus lacks leaves
	// nothing to show.
	if len(live) == 0 || len(terms) <= 2 && len(dead) > 0 {
		return []DocHit{}, dead
	}
	liveIDF := 0.0
	for _, t := range live {
		liveIDF += idf[t]
	}
	queryWeight := liveIDF + float64(len(dead))*math.Log(1+n)
	var hits []DocHit
	for _, d := range docs {
		covered := map[int]bool{}
		field := 0.0
		for _, t := range live {
			inTitle := occurrences(d.title, terms[t]) > 0
			inPath := occurrences(d.path, terms[t]) > 0
			if inTitle {
				field += titleWeight * idf[t]
			}
			if inPath {
				field += pathWeight * idf[t]
			}
			if inTitle || inPath {
				covered[t] = true
			}
		}
		bestSec, bestScore := -1, 0.0
		bestLine := 0
		for s, lines := range d.lines {
			score := 0.0
			for _, t := range live {
				if occurrences(d.headings[s], terms[t]) > 0 {
					score += headingWeight * idf[t]
				}
			}
			counts := make([]int, len(terms))
			lineIdx, lineScore := 0, 0.0
			for i, line := range lines {
				ls := 0.0
				for _, t := range live {
					if c := occurrences(line, terms[t]); c > 0 {
						counts[t] += c
						covered[t] = true
						ls += idf[t]
					}
				}
				// Prefer a body line over the heading line itself when they tie.
				if ls > lineScore || (ls == lineScore && ls > 0 && lineIdx == 0 && i > 0 && d.doc.Sections[s].Heading != "") {
					lineIdx, lineScore = i, ls
				}
			}
			for _, t := range live {
				if counts[t] > 0 {
					score += idf[t] * (1 + math.Log(float64(counts[t])))
				}
			}
			// Words that meet on one line are stronger evidence than the same words
			// scattered over a long section, and that line is the one shown.
			score += lineWeight * lineScore
			if score > bestScore {
				bestSec, bestScore = s, score
				bestLine = lineIdx
			}
		}
		coveredIDF := 0.0
		var missing []string
		for _, t := range live {
			if covered[t] {
				coveredIDF += idf[t]
			} else {
				missing = append(missing, terms[t].Word)
			}
		}
		// A one- or two-word question needs every word the corpus knows; a longer one
		// half of their weight, since a question carries words a document need not repeat.
		if len(live) <= 2 && len(missing) > 0 || coveredIDF*2 < liveIDF {
			continue
		}
		hit := DocHit{Path: d.doc.Path, Title: d.doc.Title, Missing: missing, Coverage: coveredIDF / queryWeight,
			Score: bestScore + field + 2*coveredIDF}
		if bestSec >= 0 {
			sec := d.doc.Sections[bestSec]
			hit.Heading = sec.Heading
			hit.Line = sec.Line + bestLine
			hit.Excerpt = Trim(sec.Lines[bestLine], 200)
		}
		hits = append(hits, hit)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Path < hits[j].Path
	})
	if hits == nil {
		hits = []DocHit{}
	}
	return hits, dead
}

// fuse merges ranked lists by reciprocal rank. A document keeps the hit (line,
// excerpt) from the list where it ranked best.
func fuse(lists [][]DocHit) []DocHit {
	type entry struct {
		hit  DocHit
		rank int
		sum  float64
	}
	byPath := map[string]*entry{}
	var order []string
	for _, list := range lists {
		for rank, hit := range list {
			e, ok := byPath[hit.Path]
			if !ok {
				e = &entry{hit: hit, rank: rank}
				byPath[hit.Path] = e
				order = append(order, hit.Path)
			} else if rank < e.rank {
				e.hit, e.rank = hit, rank
			}
			e.sum += 1 / float64(rrfK+rank+1)
		}
	}
	fused := make([]DocHit, 0, len(order))
	for _, p := range order {
		e := byPath[p]
		e.hit.Score = math.Round(e.sum*10000) / 10000
		fused = append(fused, e.hit)
	}
	sort.SliceStable(fused, func(i, j int) bool {
		if fused[i].Score != fused[j].Score {
			return fused[i].Score > fused[j].Score
		}
		return fused[i].Path < fused[j].Path
	})
	return fused
}
