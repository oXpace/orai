# agent-workflow-guide Specification

## Purpose

setup이 생성하는 안내가 어느 역할에게나 같은 스펙·테스트 우선 순서와 보고 형식을 주되, 승인과 완료 기준은 프로젝트 규칙에 남긴다.

## Requirements

### Requirement: 스킬이 작업 순서 참조 문서를 갖는다
`orai setup`은 Orai 스킬에 `references/workflow.md`를 생성해야 한다(SHALL). 문서는 시나리오 확인, 실패 테스트 먼저, 최소 구현, 전체 검사, 스펙과 맞추기, 보고 순서를 담고, 승인권자와 완료 기준은 프로젝트 규칙과 역할 지침에 맡긴다.

#### Scenario: setup 뒤 참조 문서
- **WHEN** `orai setup --no-tools`를 실행한다
- **THEN** `.agents/skills/orai/references/workflow.md`가 생기고 `SKILL.md` 목차와 AGENTS 블록이 그 문서를 가리킨다

### Requirement: 보고는 검증 종류를 나눈다
안내의 보고 형식은 시나리오별로 테스트 이름과 종류(fixture·fake, 실제 동작)를 나누고, 실행한 명령과 결과, 검증하지 못한 범위를 따로 적게 해야 한다(SHALL).

#### Scenario: 보고 형식 표
- **WHEN** 생성된 `workflow.md`를 읽는다
- **THEN** "시나리오별 검증", "실행한 명령", "검증하지 못한 범위" 항목이 있다

### Requirement: 역할 사이 인계는 변경 이름으로 한다
안내는 작업을 맡길 때 `--kind todo --thread change/<이름>`으로 보내고, 본문에는 첫 줄 `change: <이름> task: <tasks.md의 번호>`와 만족시킬 시나리오만 적고 스펙 내용을 옮겨 쓰지 않게 해야 한다(SHALL). 답장은 같은 첫 줄로 시작하게 하고, 메일 검색 결과는 thread가 `change/<이름>`인 메시지만 그 작업으로 보게 해야 한다(MUST).

#### Scenario: todo 인계 지침
- **WHEN** 생성된 `workflow.md`의 "역할 사이에서" 절을 읽는다
- **THEN** `todo` 본문에 변경 이름과 시나리오만 적으라는 지침이 있다
- **AND** `--thread change/<이름>`, 첫 줄 형식, `openspec instructions apply`의 순서 id를 쓰지 말라는 지침이 있다

#### Scenario: 지난 맥락 찾기
- **WHEN** 생성된 `workflow.md`에서 지난 맥락을 찾는 방법을 읽는다
- **THEN** thread가 `change/<이름>`인 메시지만 그 작업으로 보라는 지침이 있다

### Requirement: 스킬 하나가 OpenSpec 전 단계를 안내한다
Orai 스킬은 다른 스킬 없이 OpenSpec의 탐색, 제안, 계획 수정, 구현, 검증, 보관을 수행할 명령과 금지를 담아야 한다(SHALL).

#### Scenario: 단계 요청이 Orai 스킬로 온다
- **WHEN** 생성된 `SKILL.md`의 description을 읽는다
- **THEN** propose, apply, verify, sync, archive가 들어 있다

#### Scenario: 단계별 절
- **WHEN** 생성된 `workflow.md`를 읽는다
- **THEN** 탐색, 제안, 계획 수정, 구현, 검증, 보관 절이 있고 `openspec-*` 스킬을 읽으라는 지시가 없다

#### Scenario: 안전한 보관
- **WHEN** 생성된 `workflow.md`의 보관 절을 읽는다
- **THEN** 현재 스펙과의 diff 확인과 `--yes` 없는 첫 실행이 `--yes` 실행보다 앞에 있다

### Requirement: 옮겨 온 내용의 고지를 함께 배포한다
`orai setup`은 OpenSpec에서 옮겨 온 내용의 저작권과 MIT 허가 문구를 Orai 스킬과 함께 생성해야 한다(SHALL).

#### Scenario: setup 뒤 고지
- **WHEN** `orai setup --no-tools`를 실행한다
- **THEN** `.agents/skills/orai/references/openspec-license.txt`에 저작권자와 MIT 허가 문구가 있다

### Requirement: change 사이의 순서를 적는다
안내는 change 사이의 선후를 `.openspec.yaml`의 `x-after`에 적고, 선행 change가 보관된 뒤에 맡기게 해야 한다(SHALL). Orai와 OpenSpec은 이 순서를 강제하지 않는다.

#### Scenario: 선행 change
- **WHEN** 생성된 `workflow.md`의 "역할 사이에서" 절을 읽는다
- **THEN** `x-after`와 보관된 뒤에 맡긴다는 규칙이 있다

### Requirement: 맡길 권한은 역할 지침에 둔다
change를 누가 맡기는지는 Orai 스킬이 아니라 생성되는 역할 지침 템플릿에 적어야 한다(SHALL).

#### Scenario: 역할 지침
- **WHEN** manager, engineer, 일반 역할의 지침 템플릿을 읽는다
- **THEN** 각각 change를 맡기는지, 누구에게서 받는지, 또는 프로젝트가 정할 자리가 있다

### Requirement: 기획, 설계, 검증의 기록 위치를 나눈다
안내는 스펙에 요구와 동작 계약만, change에 설계·기술 결정과 구현 단계만 적고, 실검증·재검증·테스트 보강·버그는 프로젝트의 이슈 트래커에 두게 해야 한다(SHALL). 생성되는 OpenSpec 시작 규칙은 검증과 테스트를 tasks의 항목으로 요구하지 않아야 한다(MUST).

#### Scenario: 작업 흐름과 AGENTS 블록
- **WHEN** 생성된 `workflow.md`와 AGENTS 블록을 읽는다
- **THEN** 스펙, change, 이슈 트래커에 각각 무엇을 적는지 나와 있다

#### Scenario: 새 프로젝트의 시작 규칙
- **WHEN** 생성된 `openspec/config.yaml`의 tasks 규칙을 읽는다
- **THEN** 설계와 구현 단계만 적고 검증은 이슈로 보내라는 규칙이 있다
