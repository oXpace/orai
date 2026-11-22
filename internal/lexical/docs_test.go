package lexical

import (
	"reflect"
	"testing"
)

func TestParseMarkdownSplitsAtHeadingsOutsideCode(t *testing.T) {
	doc := ParseMarkdown("docs/a.md", "---\ntitle: \"설계\"\n---\n머리말\n# 큰 제목\n본문\n```\n# 코드 안 주석\n```\n## 절 둘\n끝\n")
	if doc.Title != "설계" {
		t.Fatalf("front matter title wins: %q", doc.Title)
	}
	var heads []string
	var lines []int
	for _, s := range doc.Sections {
		heads, lines = append(heads, s.Heading), append(lines, s.Line)
	}
	if !reflect.DeepEqual(heads, []string{"", "큰 제목", "절 둘"}) || !reflect.DeepEqual(lines, []int{4, 5, 10}) {
		t.Fatalf("sections = %v at %v", heads, lines)
	}
	if got := ParseMarkdown("notes/plan.md", "# 계획\n").Title; got != "계획" {
		t.Fatalf("first H1 is the title: %q", got)
	}
	if got := ParseMarkdown("notes/plan.md", "본문만\n").Title; got != "plan" {
		t.Fatalf("file name is the last resort: %q", got)
	}
}

func TestVariantsTrimHangulTailsAndSplitMixedWords(t *testing.T) {
	for word, want := range map[string][]string{
		"신원을":     {"신원을", "신원"},
		"컴포넌트들을":  {"컴포넌트들을", "컴포넌트들", "컴포넌트"},
		"hook이":   {"hook이", "hook"},
		"세션":      {"세션"},
		"release": {"release"},
	} {
		if got := variants(word); !reflect.DeepEqual(got, want) {
			t.Errorf("variants(%q) = %v, want %v", word, got, want)
		}
	}
}

// corpus: the word every document shares ("문서") must not decide the ranking; the rare
// word ("nonce") must, and the line where the words meet is the one shown.
func corpus() []Doc {
	return []Doc{
		ParseMarkdown("docs/a.md", "# 개요\n문서 문서 문서 문서\n문서 설명\n"),
		ParseMarkdown("docs/b.md", "# 캡처\n문서 앞부분\n\n## 신원\n관계없는 줄\n문서의 nonce가 다르면 거부한다\n"),
		ParseMarkdown("docs/c.md", "# 기타\n문서\n"),
	}
}

func TestSearchDocsRanksByRareWordsAndPointsAtTheSectionLine(t *testing.T) {
	r := SearchDocs(corpus(), []string{"문서 nonce"}, 0)
	if len(r.Hits) != 1 || r.Weak {
		t.Fatalf("result = %+v", r)
	}
	hit := r.Hits[0]
	if hit.Path != "docs/b.md" || hit.Heading != "신원" || hit.Line != 6 || hit.Excerpt != "문서의 nonce가 다르면 거부한다" {
		t.Fatalf("hit = %+v", hit)
	}
}

func TestSearchDocsWeighsTheTitleAndHeading(t *testing.T) {
	docs := []Doc{
		ParseMarkdown("docs/body.md", "# 잡다한 것\n배포 배포 배포 이야기\n"),
		ParseMarkdown("docs/deploy.md", "# 배포\n절차는 여기\n"),
		ParseMarkdown("docs/x.md", "# 다른 것\n내용\n"),
	}
	r := SearchDocs(docs, []string{"배포"}, 0)
	if len(r.Hits) != 2 || r.Hits[0].Path != "docs/deploy.md" {
		t.Fatalf("a title hit outranks repetition in the body: %+v", r.Hits)
	}
}

func TestSearchDocsFusesPhrasingsAndSaysWhenWeak(t *testing.T) {
	docs := []Doc{
		ParseMarkdown("docs/port.md", "# 포트\n기본 포트는 ID에서 유도한다\n"),
		ParseMarkdown("docs/other.md", "# 질문 모음\n어떻게 하나요\n"),
		ParseMarkdown("docs/z.md", "# 무관\n내용\n"),
	}
	one := SearchDocs(docs, []string{"번호를 어떻게 정하나요 zzqx"}, 0)
	if !one.Weak || !reflect.DeepEqual(one.Unknown, []string{"zzqx", "번호를", "정하나요"}) {
		t.Fatalf("a question whose answer uses other words is weak: %+v", one)
	}
	fused := SearchDocs(docs, []string{"번호를 어떻게 정하나요 zzqx", "포트 유도", "기본 포트"}, 0)
	if len(fused.Hits) == 0 || fused.Hits[0].Path != "docs/port.md" || fused.Weak || len(fused.Unknown) != 0 {
		t.Fatalf("phrasings that agree put the answer first: %+v", fused)
	}
	if fused.Hits[0].Line != 2 {
		t.Fatalf("the fused hit keeps its best phrasing's line: %+v", fused.Hits[0])
	}
}

func TestSearchDocsShortQueriesNeedEveryWord(t *testing.T) {
	if r := SearchDocs(corpus(), []string{"zzqx 문서"}, 0); len(r.Hits) != 0 || !r.Weak {
		t.Fatalf("a two-word query with an unknown word shows nothing: %+v", r)
	}
	if r := SearchDocs(corpus(), []string{"설명 nonce"}, 0); len(r.Hits) != 0 {
		t.Fatalf("no document has both words: %+v", r)
	}
}
