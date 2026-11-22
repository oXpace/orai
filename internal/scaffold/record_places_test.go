package scaffold_test

import (
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/scaffold"
)

// 시나리오: 작업 흐름과 AGENTS 블록
func TestGuidanceSeparatesPlansFromVerification(t *testing.T) {
	for _, name := range []string{"skill-workflow.md", "agents-block.md"} {
		text := scaffold.Template(name)
		for _, want := range []string{"요구와 동작 계약", "설계·기술 결정과 구현 단계", "실검증·재검증·테스트 보강·버그", "이슈 트래커"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks %q", name, want)
			}
		}
	}
	if strings.Contains(scaffold.Template("skill-workflow.md"), "묶음마다 테스트와 문서를 포함") {
		t.Error("the workflow still asks for tests as tasks")
	}
}

// 시나리오: 새 프로젝트의 시작 규칙
func TestStarterKeepsVerificationOutOfTasks(t *testing.T) {
	_, rules, _ := strings.Cut(scaffold.Template("openspec-config.yaml"), "\n  tasks:\n")
	rules, _, _ = strings.Cut(rules, "\noperations:")
	if !strings.Contains(rules, "이슈 트래커") || strings.Contains(rules, "테스트 작성을 첫 작업") {
		t.Fatalf("tasks rules:\n%s", rules)
	}
}
