package scaffold_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/scaffold"
)

// 시나리오: 단계 요청이 Orai 스킬로 온다
func TestSkillDescriptionCoversTheOpenSpecSteps(t *testing.T) {
	text := scaffold.Template("skill.md")
	description := regexp.MustCompile(`(?m)^description: .*$`).FindString(text)
	for _, word := range []string{"propose", "apply", "verify", "sync", "archive"} {
		if !strings.Contains(description, word) {
			t.Errorf("description lacks %q", word)
		}
	}
}

// 시나리오: 단계별 절
func TestWorkflowCoversEveryStepWithoutOtherSkills(t *testing.T) {
	text := scaffold.Template("skill-workflow.md")
	for _, step := range []string{"탐색", "제안", "계획 수정", "구현", "검증", "보관"} {
		if !strings.Contains(text, "\n## "+step+"\n") {
			t.Errorf("no section %q", step)
		}
	}
	named := regexp.MustCompile("openspec-(explore|propose|apply-change|update-change|verify-change|sync-specs|archive-change)")
	for _, name := range []string{"skill-workflow.md", "skill.md", "manager.md", "engineer.md", "role.md", "agents-block.md"} {
		if found := named.FindString(scaffold.Template(name)); found != "" {
			t.Errorf("%s sends the reader to %s", name, found)
		}
	}
}

// 시나리오: 안전한 보관
func TestArchiveChecksTheDiffAndRunsWithoutYesFirst(t *testing.T) {
	_, section, found := strings.Cut(scaffold.Template("skill-workflow.md"), "\n## 보관\n")
	if !found {
		t.Fatal("no archive section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	diff := strings.Index(section, "--diff")
	first := strings.Index(section, "`openspec archive <변경> --json`")
	yes := strings.Index(section, "`openspec archive <변경> --json --yes`")
	if diff < 0 || first < diff || yes < first {
		t.Fatalf("order: diff %d, first run %d, --yes %d", diff, first, yes)
	}
}

// 시나리오: setup 뒤 고지
func TestSkillShipsTheOpenSpecLicense(t *testing.T) {
	for _, file := range scaffold.SkillFiles {
		if file.Rel != scaffold.SkillDir+"/references/openspec-license.txt" {
			continue
		}
		text := scaffold.Template(file.Template)
		for _, want := range []string{scaffold.Generated, "MIT License", "Copyright (c) 2025 FISSION AI INC", "Permission is hereby granted"} {
			if !strings.Contains(text, want) {
				t.Errorf("notice lacks %q", want)
			}
		}
		return
	}
	t.Fatal("setup does not generate the notice")
}
