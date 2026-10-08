package shelf

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

// lookPath resolves a binary on PATH; tests swap it so a missing/present "qmd" can be
// simulated without touching the real PATH.
var lookPath = exec.LookPath

// Diagnose is the read-only, layered shelf diagnosis: not-configured → engine on PATH →
// collection folders exist → project initialized → probe() the running server.
func Diagnose(p *project.Project, deep bool) []doctor.Check {
	if p.Config.Shelf == nil {
		return []doctor.Check{doctor.New("shelf", doctor.NotConfigured, "no [integrations.shelf] in orai.toml", "")}
	}
	s := NewSettings(p)
	// The index and server first, then what the project's own MCP settings say about it.
	return append(diagnose(s, deep), registration(s, deep))
}

func diagnose(s *Settings, deep bool) []doctor.Check {
	if _, err := lookPath("qmd"); err != nil {
		return []doctor.Check{doctor.New("shelf", doctor.Blocked, "the shelf engine QMD (qmd) is not on PATH", InstallHint)}
	}
	var missing []string
	for _, name := range s.collectionNames() {
		info, err := os.Stat(s.Collections[name])
		if err != nil || !info.IsDir() {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return []doctor.Check{doctor.New("shelf", doctor.NotReady,
			fmt.Sprintf("collection folder(s) missing: %s", strings.Join(missing, ", ")),
			"Create the folder(s) or fix integrations.shelf.collections in orai.toml")}
	}
	if s.legacyOnly() {
		return []doctor.Check{doctor.New("shelf", doctor.NotReady,
			"the index is still where Orai 0.3 kept it (.orai/wiki)",
			"`orai shelf recover` moves it to .orai/shelf and restarts the server; nothing is rebuilt")}
	}
	if !exists(s.ConfigFile) || !exists(s.DB) {
		c := doctor.New("shelf", doctor.NotReady, "the shelf has not been built for this project yet",
			"`orai shelf init` (the first run downloads the embedding model, about 0.6 GB)")
		return []doctor.Check{c.WithDetail(map[string]any{"version": doctor.VersionOf("qmd")})}
	}
	return probe(s, deep)
}

// probe runs the layered checks: reachable → MCP handshake → identity → index →
// (deep) vector → hybrid → get. It is a var so tests can substitute canned results
// (for example to test verify()'s failure handling without a fake server).
var probe = func(s *Settings, deep bool) []doctor.Check {
	c := "shelf"
	state, detailErr := reachability(s.Port)
	detail := map[string]any{"engine": "qmd", "endpoint": s.Endpoint, "index": s.Index, "db": s.DB}
	switch state {
	case "denied":
		return []doctor.Check{doctor.New(c+".server", doctor.Blocked,
			fmt.Sprintf("access to %s denied in this environment (%s); this does not show the server is down",
				s.Endpoint, detailErr),
			"Run the check outside the sandbox; a result there applies only to that environment").WithDetail(detail)}
	case "refused":
		return []doctor.Check{doctor.New(c+".server", doctor.Blocked,
			"the shelf server is not running (it does not survive a reboot)", "`orai shelf recover`").WithDetail(detail)}
	case "error":
		return []doctor.Check{doctor.New(c+".server", doctor.Blocked,
			fmt.Sprintf("port probe failed: %s", detailErr),
			fmt.Sprintf("Check that 127.0.0.1:%d is usable on this machine, or set integrations.shelf.port in orai.toml to another port", s.Port)).WithDetail(detail)}
	}
	checks := []doctor.Check{doctor.New(c+".server", doctor.Healthy, "port accepts connections", "").WithDetail(detail)}

	timeout := 15 * time.Second
	if deep {
		timeout = 180 * time.Second
	}
	client := newClient(s.Endpoint, timeout)
	status, err := handshake(client)
	if err != nil {
		checks = append(checks, doctor.New(c+".mcp", doctor.Blocked,
			fmt.Sprintf("MCP handshake/status failed: %v", err), "Check the QMD log ("+withSuffix(s.PIDFile(), ".log")+"), then `orai shelf stop && orai shelf recover`"))
		return checks
	}
	checks = append(checks, doctor.New(c+".mcp", doctor.Healthy, "MCP initialize and status succeeded", ""))

	if mismatch := identity(status, s); mismatch != "" {
		checks = append(checks, doctor.New(c+".identity", doctor.Blocked,
			fmt.Sprintf("port %d serves another index: %s", s.Port, mismatch),
			"That server is left untouched. Set integrations.shelf.port in orai.toml to a free port, then `orai shelf recover`"))
		return checks
	}
	checks = append(checks, doctor.New(c+".identity", doctor.Healthy, "server indexes this project's collections", ""))

	// What is indexed should be what orai.toml declares, once each.
	if extra := undeclared(status, s); len(extra) > 0 {
		checks = append(checks, doctor.New(c+".collections", doctor.Degraded,
			fmt.Sprintf("the index still has collection(s) orai.toml no longer declares: %s (their documents still show up in searches)", strings.Join(extra, ", ")),
			"`orai shelf stop && orai shelf refresh`"))
	} else if pairs := s.overlapping(); len(pairs) > 0 {
		checks = append(checks, doctor.New(c+".collections", doctor.Degraded,
			fmt.Sprintf("collections %s cover the same documents, so searches return them twice", strings.Join(pairs, "; ")),
			overlapFix+" in orai.toml, then `orai shelf stop && orai shelf refresh`"))
	}

	if check, declared := contextCheck(s); declared {
		checks = append(checks, check)
	}

	if !truthy(status["totalDocuments"]) {
		checks = append(checks, doctor.New(c+".index", doctor.NotReady, "the shelf index has no documents",
			"Add Markdown files under "+strings.Join(s.collectionNames(), ", ")+", then `orai shelf sync`"))
		return checks
	}
	if !truthy(status["hasVectorIndex"]) || truthy(status["needsEmbedding"]) {
		chk := doctor.New(c+".index", doctor.Degraded, "some documents are not embedded yet (edited since the last sync)",
			"`orai shelf sync`")
		checks = append(checks, chk.WithDetail(map[string]any{"needsEmbedding": status["needsEmbedding"]}))
		return checks
	}
	checks = append(checks, doctor.New(c+".index", doctor.Healthy,
		fmt.Sprintf("%v documents with vectors", status["totalDocuments"]), ""))

	if !deep {
		checks = append(checks, doctor.New(c+".search", doctor.NotChecked,
			"search itself was not run (it loads the search models)", "`orai doctor --deep` runs real searches"))
		return checks
	}
	checks = append(checks, searchChecks(client, s)...)
	return checks
}

// handshake performs the MCP initialize/status round-trip probe() and
// ownServerRunning() both need, returning the `status` tool's structuredContent.
func handshake(client mcpClient) (map[string]any, error) {
	if err := client.Connect(); err != nil {
		return nil, err
	}
	result, err := client.Tool("status", nil)
	if err != nil {
		return nil, err
	}
	status, _ := result["structuredContent"].(map[string]any)
	if status == nil {
		status = map[string]any{}
	}
	return status, nil
}

// searchChecks exercises the models: vector-only first (so a lexical fallback cannot
// hide an embedding failure), then lex+vec plus a document read.
func searchChecks(client mcpClient, s *Settings) []doctor.Check {
	c := "shelf"
	names := s.collectionNames()
	smoke := s.Smoke
	var expect string
	if smoke != nil {
		expect = expectedURI(s)
	}
	vecQuery := "project overview and architecture"
	if smoke != nil {
		vecQuery = smoke.Vec
	}

	hits := func(searches []map[string]any) ([]string, error) {
		result, err := client.Tool("query", map[string]any{"searches": searches, "collections": names, "limit": 5})
		if err != nil {
			return nil, err
		}
		structured, _ := result["structuredContent"].(map[string]any)
		results, _ := structured["results"].([]any)
		files := make([]string, 0, len(results))
		for _, r := range results {
			row, _ := r.(map[string]any)
			file, _ := row["file"].(string)
			files = append(files, file)
		}
		return files, nil
	}

	vector, err := hits([]map[string]any{{"type": "vec", "query": vecQuery}})
	if err != nil {
		return []doctor.Check{doctor.New(c+".vector", doctor.Blocked,
			fmt.Sprintf("vector search failed: %v", err), "Check model/GPU availability in the QMD log")}
	}
	if len(vector) == 0 {
		return []doctor.Check{doctor.New(c+".vector", doctor.Degraded, "vector search returned no results", "`orai shelf sync`")}
	}

	var checks []doctor.Check
	matched := false
	if expect != "" {
		for _, h := range vector {
			if documentKey(h) == documentKey(expect) {
				matched = true
				break
			}
		}
	}
	if expect != "" && !matched {
		chk := doctor.New(c+".vector", doctor.Degraded,
			fmt.Sprintf("vector search missed the smoke document %s", expect),
			"Check [integrations.shelf.smoke] in orai.toml (vec should be a question that document answers); if it is right, the embedding model recalls poorly for these documents")
		checks = append(checks, chk.WithDetail(map[string]any{"hits": vector}))
	} else {
		reason := "vector-only search returned results (set [integrations.shelf.smoke] to check it returns the right document)"
		if expect != "" {
			reason = "vector-only search returned the smoke document"
		}
		checks = append(checks, doctor.New(c+".vector", doctor.Healthy, reason, ""))
	}

	lex := "overview"
	if smoke != nil {
		lex = smoke.Lex
	}
	hybrid, err := hits([]map[string]any{{"type": "lex", "query": lex}, {"type": "vec", "query": vecQuery}})
	var body string
	if err == nil && len(hybrid) == 0 {
		err = mcpErrorf("no results")
	}
	if err == nil {
		file := expect
		if file == "" {
			file = hybrid[0]
		}
		var result map[string]any
		result, err = client.Tool("get", map[string]any{"file": file, "maxLines": 20})
		if err == nil {
			body = textOf(result)
		}
	}
	if err != nil {
		checks = append(checks, doctor.New(c+".hybrid", doctor.Blocked,
			fmt.Sprintf("lex+vec search or document read failed: %v", err),
			"Check the QMD log ("+withSuffix(s.PIDFile(), ".log")+"), then `orai shelf stop && orai shelf recover`"))
		return checks
	}
	if strings.TrimSpace(body) != "" {
		checks = append(checks, doctor.New(c+".hybrid", doctor.Healthy, "lex+vec search and document read succeeded", ""))
	} else {
		checks = append(checks, doctor.New(c+".hybrid", doctor.Degraded, "document read returned no text",
			"`orai shelf sync` to bring the index up to the current documents"))
	}
	return checks
}
