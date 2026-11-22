package scaffold_test

import (
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/scaffold"
)

// 시나리오: todo 인계 지침 / 지난 맥락 찾기 / 선행 change
func TestWorkflowAssignsWorkByChangeThread(t *testing.T) {
	_, section, found := strings.Cut(scaffold.Template("skill-workflow.md"), "\n## 역할 사이에서\n")
	if !found {
		t.Fatal("no hand-off section")
	}
	for _, want := range []string{
		"--kind todo --thread change/<이름>",
		"`change: <이름> task: 1.2`",
		"순서 id",
		"thread가 `change/<이름>`인 메시지만",
		"`x-after`",
		"보관된 뒤",
		"시나리오만",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("hand-off lacks %q", want)
		}
	}
}

// 시나리오: 역할 지침
func TestRoleGuidesSayWhoAssignsChanges(t *testing.T) {
	for name, want := range map[string]string{
		"manager.md":  "change를 맡기는 일은 이 역할이 한다",
		"engineer.md": "change는 `{{manager}}` 역할에게서 받는다",
		"role.md":     "(change를 다른 역할에 맡기는지, 누구에게서 받는지)",
	} {
		if !strings.Contains(scaffold.Template(name), want) {
			t.Errorf("%s lacks %q", name, want)
		}
	}
}
