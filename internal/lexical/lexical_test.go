package lexical

import "testing"

func TestTermsLowercaseDeduplicateAndStemKorean(t *testing.T) {
	terms := Terms("Hook 세션 신원을 hook, a")
	if len(terms) != 3 {
		t.Fatalf("terms = %+v, want hook, 세션, 신원을", terms)
	}
	if terms[0].Word != "hook" || terms[2].Word != "신원을" || len(terms[2].Forms) != 2 || terms[2].Forms[1] != "신원" {
		t.Fatalf("terms = %+v", terms)
	}
	if len(terms[1].Forms) != 1 {
		t.Fatalf("a two-syllable word keeps its only form: %+v", terms[1])
	}
}

func TestScoreFindsTheBestLineAndTheStemMatches(t *testing.T) {
	text := "첫 줄\n낡은 hook은 신원이 다르다\n관계없음"
	terms := Terms("낡은 hook이 신원을 덮는다")
	m := Score(text, terms)
	if m.Line != 2 || m.Distinct < 3 || !m.Enough(terms) {
		t.Fatalf("match = %+v", m)
	}
	if Score("nothing here", terms).Enough(terms) {
		t.Fatal("an unrelated text must not be enough")
	}
}

func TestShortQueriesNeedEveryTerm(t *testing.T) {
	terms := Terms("alpha beta")
	if Score("alpha only", terms).Enough(terms) {
		t.Fatal("a two-word query needs both words")
	}
	if !Score("alpha and beta", terms).Enough(terms) {
		t.Fatal("both words present must be enough")
	}
}
