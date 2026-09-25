package wiki

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

// Diagnose is the read-only, layered wiki diagnosis: not-configured → engine on PATH →
// collection folders exist → project initialized → probe() the running server.
func Diagnose(p *project.Project, deep bool) []doctor.Check {
	if p.Config.Wiki == nil {
		return []doctor.Check{doctor.New("wiki", doctor.NotConfigured, "no [integrations.wiki] in orai.toml", "")}
	}
	s := NewSettings(p)
	if _, err := lookPath("qmd"); err != nil {
		return []doctor.Check{doctor.New("wiki", doctor.Blocked, "wiki engine qmd is not on PATH", InstallHint)}
	}
	var missing []string
	for _, name := range s.collectionNames() {
		info, err := os.Stat(s.Collections[name])
		if err != nil || !info.IsDir() {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return []doctor.Check{doctor.New("wiki", doctor.NotReady,
			fmt.Sprintf("collection folder(s) missing: %s", strings.Join(missing, ", ")),
			"Create the folders or fix integrations.wiki.collections")}
	}
	if !exists(s.ConfigFile) || !exists(s.DB) {
		c := doctor.New("wiki", doctor.NotReady, "project index not initialized", "orai wiki init")
		return []doctor.Check{c.WithDetail(map[string]any{"version": doctor.VersionOf("qmd")})}
	}
	return probe(s, deep)
}

// probe runs the layered checks: reachable → MCP handshake → identity → index →
// (deep) vector → hybrid → get. It is a var so tests can substitute canned results
// (for example to test verify()'s failure handling without a fake server).
var probe = func(s *Settings, deep bool) []doctor.Check {
	c := "wiki"
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
			"connection refused: no server on the project port", "orai wiki recover").WithDetail(detail)}
	case "error":
		return []doctor.Check{doctor.New(c+".server", doctor.Blocked,
			fmt.Sprintf("port probe failed: %s", detailErr), "").WithDetail(detail)}
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
			fmt.Sprintf("MCP handshake/status failed: %v", err), "Check the QMD log, then orai wiki recover"))
		return checks
	}
	checks = append(checks, doctor.New(c+".mcp", doctor.Healthy, "MCP initialize and status succeeded", ""))

	if mismatch := identity(status, s); mismatch != "" {
		checks = append(checks, doctor.New(c+".identity", doctor.Blocked,
			fmt.Sprintf("port %d serves another index: %s", s.Port, mismatch),
			"Left untouched. Set integrations.wiki.port to a free port or stop that server"))
		return checks
	}
	checks = append(checks, doctor.New(c+".identity", doctor.Healthy, "server indexes this project's collections", ""))

	if !truthy(status["totalDocuments"]) {
		checks = append(checks, doctor.New(c+".index", doctor.NotReady, "index is empty", "Add Markdown docs, then orai wiki refresh"))
		return checks
	}
	if !truthy(status["hasVectorIndex"]) || truthy(status["needsEmbedding"]) {
		chk := doctor.New(c+".index", doctor.Degraded, "embeddings missing or incomplete", "orai wiki refresh")
		checks = append(checks, chk.WithDetail(map[string]any{"needsEmbedding": status["needsEmbedding"]}))
		return checks
	}
	checks = append(checks, doctor.New(c+".index", doctor.Healthy,
		fmt.Sprintf("%v documents with vectors", status["totalDocuments"]), ""))

	if !deep {
		checks = append(checks, doctor.New(c+".search", doctor.NotChecked,
			"semantic search not exercised (loads models)", "orai doctor --deep"))
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
	c := "wiki"
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
		return []doctor.Check{doctor.New(c+".vector", doctor.Degraded, "vector search returned no results", "orai wiki refresh")}
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
			"Review recall for this model (docs/operations.md)")
		checks = append(checks, chk.WithDetail(map[string]any{"hits": vector}))
	} else {
		reason := "vector-only search returned results (no smoke fixture configured)"
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
			fmt.Sprintf("lex+vec search or document read failed: %v", err), ""))
		return checks
	}
	if strings.TrimSpace(body) != "" {
		checks = append(checks, doctor.New(c+".hybrid", doctor.Healthy, "lex+vec search and document read succeeded", ""))
	} else {
		checks = append(checks, doctor.New(c+".hybrid", doctor.Degraded, "document read returned no text", ""))
	}
	return checks
}
