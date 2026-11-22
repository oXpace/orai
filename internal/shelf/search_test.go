package shelf

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func writeDoc(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMatchPatternHandlesDoubleStar(t *testing.T) {
	for _, c := range []struct {
		pattern, path string
		want          bool
	}{
		{"**/*.md", "a.md", true}, {"**/*.md", "x/y/a.md", true}, {"**/*.md", "a.txt", false},
		{"*.md", "a.md", true}, {"*.md", "x/a.md", false}, {"adr/**/*.md", "adr/1/a.md", true},
	} {
		if got := MatchPattern(c.pattern, c.path); got != c.want {
			t.Errorf("MatchPattern(%q, %q) = %v", c.pattern, c.path, got)
		}
	}
}

func TestSearchFallsBackToKeywordsWithTheReasonWhenNotBuilt(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 18555)
	writeDoc(t, filepath.Join(p.Root, "docs", "deploy.md"), "# 배포\n\n배포는 release 절차를 따른다\n")
	writeDoc(t, filepath.Join(p.Root, "docs", "other.md"), "# 기타\n")
	out := Search(NewSettings(p), "release 절차", 5)
	if out.Mode != "keyword" || out.Fix != "orai shelf init" || out.Reason == "" {
		t.Fatalf("outcome = %+v", out)
	}
	if len(out.Hits) != 1 || out.Hits[0].Path != "docs/deploy.md" || out.Hits[0].Line != 3 || out.Hits[0].Title != "배포" {
		t.Fatalf("hits = %+v", out.Hits)
	}
	if out.Weak {
		t.Fatalf("every word matched, so the result is not weak: %+v", out)
	}
	weak := Search(NewSettings(p), "zzqx 절차", 5)
	if len(weak.Hits) != 0 || !weak.Weak || len(weak.Unknown) != 1 || weak.Unknown[0] != "zzqx" {
		t.Fatalf("an unknown word is reported: %+v", weak)
	}
	fused := Search(NewSettings(p), "zzqx 절차", 5, "release")
	if len(fused.Hits) != 1 || fused.Hits[0].Path != "docs/deploy.md" {
		t.Fatalf("another phrasing finds it: %+v", fused)
	}
}

func TestSearchUsesTheEngineAndMapsPathsToTheProject(t *testing.T) {
	s := qmdSettings(t, false, 18556)
	writeDoc(t, filepath.Join(s.Project.Root, "docs", "a.md"), "x")
	writeDoc(t, s.DB, "db")
	writeDoc(t, s.SyncedFile(), "now")
	saved := reachability
	t.Cleanup(func() { reachability = saved })
	reachability = func(int) (string, string) { return "open", "" }
	var calls []toolCall
	setClient(t, &fakeClient{
		calls: &calls,
		status: func() map[string]any {
			return map[string]any{"structuredContent": map[string]any{"collections": []any{
				map[string]any{"name": "docs", "path": s.Collections["docs"]}}}}
		},
		query: func(map[string]any) map[string]any {
			return map[string]any{"structuredContent": map[string]any{"results": []any{map[string]any{
				"file": "docs/a.md", "title": "A", "score": 0.8, "context": "설계 문서", "line": float64(4), "snippet": "4: 내용"}}}}
		},
	})
	out := Search(s, "질문", 3, "다른 표현")
	if out.Mode != "engine" || len(out.Hits) != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	hit := out.Hits[0]
	if hit.Path != "docs/a.md" || hit.Line != 4 || hit.Context != "설계 문서" || hit.Excerpt != "4: 내용" {
		t.Fatalf("hit = %+v", hit)
	}
	searches := calls[len(calls)-1].Args["searches"].([]map[string]any)
	if len(searches) != 4 || searches[0]["type"] != "lex" || searches[1]["type"] != "vec" || searches[3]["query"] != "다른 표현" {
		t.Fatalf("query args = %+v", calls[len(calls)-1].Args)
	}
}

func TestSearchReportsAServerThatIsDownAndDocumentsChangedSinceSync(t *testing.T) {
	s := qmdSettings(t, false, 18557)
	writeDoc(t, s.DB, "db")
	writeDoc(t, s.SyncedFile(), "then")
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(s.SyncedFile(), old, old)
	writeDoc(t, filepath.Join(s.Project.Root, "docs", "new.md"), "새 문서")
	saved := reachability
	t.Cleanup(func() { reachability = saved })
	reachability = func(int) (string, string) { return "refused", "" }
	out := Search(s, "새 문서", 3)
	if out.Mode != "keyword" || out.Fix != "orai shelf recover" {
		t.Fatalf("outcome = %+v", out)
	}
	if len(out.Stale) != 1 || out.Stale[0] != "docs/new.md" {
		t.Fatalf("stale = %v", out.Stale)
	}
}

func TestAutoSyncSyncsAfterADocumentChangesAndNotForOtherFiles(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 18558)
	s := NewSettings(p)
	writeDoc(t, s.DB, "db")
	writeDoc(t, s.ConfigFile, "{}")
	writeDoc(t, s.SyncedFile(), "now")
	savedLook, savedRun, savedDebounce := lookPath, autoSyncRun, autoSyncDebounce
	t.Cleanup(func() { lookPath, autoSyncRun, autoSyncDebounce = savedLook, savedRun, savedDebounce })
	lookPath = func(string) (string, error) { return "/bin/true", nil }
	autoSyncDebounce = 50 * time.Millisecond
	var runs atomic.Int32
	autoSyncRun = func(s *Settings, _ io.Writer) error {
		runs.Add(1)
		return os.WriteFile(s.SyncedFile(), []byte("later"), 0o600)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { AutoSync(ctx, p); close(done) }()
	defer func() { cancel(); <-done }()
	time.Sleep(200 * time.Millisecond) // watcher up, nothing stale at start
	if runs.Load() != 0 {
		t.Fatalf("synced with nothing changed: %d", runs.Load())
	}
	writeDoc(t, filepath.Join(p.Root, "docs", "notes.txt"), "not a document")
	time.Sleep(200 * time.Millisecond)
	if runs.Load() != 0 {
		t.Fatal("a file outside the pattern triggered a sync")
	}
	writeDoc(t, filepath.Join(p.Root, "docs", "sub", "a.md"), "# 새 문서")
	deadline := time.Now().Add(3 * time.Second)
	for runs.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if runs.Load() != 1 {
		t.Fatalf("runs = %d, want one sync after the edit", runs.Load())
	}
}

func TestAutoSyncNeverBuildsAShelfThatWasNotBuilt(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 18559)
	savedLook, savedRun := lookPath, autoSyncRun
	t.Cleanup(func() { lookPath, autoSyncRun = savedLook, savedRun })
	lookPath = func(string) (string, error) { return "/bin/true", nil }
	autoSyncRun = func(*Settings, io.Writer) error { t.Error("synced an unbuilt shelf"); return nil }
	done := make(chan struct{})
	go func() { AutoSync(context.Background(), p); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("AutoSync should return at once without an index")
	}
}
