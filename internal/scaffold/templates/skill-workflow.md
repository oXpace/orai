<!-- orai:generated — `orai setup`이 관리한다. 프로젝트 규칙은 AGENTS.md나 역할 지침에 둔다. -->
<!-- OpenSpec 2.1.1의 스킬과 spec-driven 스키마를 줄여 옮긴 내용을 포함한다(MIT). 고지: openspec-license.txt -->

# 스펙과 테스트로 일하기

역할이 없어도 쓴다. 기능을 만들거나 동작을 바꿀 때, 작업을 넘겨받았을 때, 보고하기 전에 해당 절만 읽는다. 누가 승인하고 언제 완료로 보는지는 프로젝트 규칙과 역할 지침이 정한다.

`openspec/`가 없으면 OpenSpec 절은 건너뛰고 "테스트 우선", "보고 형식", "역할 사이에서"만 따른다. `openspec/`를 임의로 만들지 않는다. `openspec-*` 스킬이 함께 보여도 이 프로젝트에서는 이 문서의 절차를 따른다.

## 지금 할 단계

| 상황 | 단계 |
|---|---|
| 무엇을 만들지 정해지지 않았다 | 탐색 |
| 동작을 추가·변경·삭제한다 | 제안 |
| 동작이 그대로인 큰 작업(리팩터링, 도구, 검증) | 제안 (`skip_specs: true`) |
| 기존 스펙대로 되돌리는 작은 버그 수정 | change 없이 테스트 우선 |
| 진행 중인 change의 계획이 바뀌었다 | 계획 수정. 목적 자체가 바뀌었으면 새 change |
| 구현을 요청받았고 tasks가 있다 | 구현 |
| 구현이 끝났다 | 검증 |
| 검증이 끝났고 보관을 요청받았거나 프로젝트 규칙이 보관하라고 한다 | 보관 |

## specs와 changes

| 위치 | 뜻 | 고치는 때 |
|---|---|---|
| `openspec/specs/<기능>/spec.md` | 현재 동작의 계약 | 보관할 때 CLI가 고친다. Purpose 수정만 직접 한다 |
| `openspec/changes/<변경>/` | 아직 보관하지 않은 변경: proposal, specs 델타, design, tasks | 제안부터 보관 전까지 |
| `openspec/changes/archive/` | 끝난 변경의 기록 | 고치지 않는다 |
| `openspec/config.yaml` | 프로젝트의 `context`, `rules`, `operations` | 프로젝트 규칙을 바꿀 때 |

- 현재 동작: `orai search '<질문>' --in docs`로 찾고 `orai read <경로>`로 읽는다. 목록은 `openspec list --specs`.
- 진행 중인 변경: `openspec list`, `openspec show <변경> --type change`. 문서 검색에는 나오지 않을 수 있다.
- 산출물의 형식과 프로젝트 규칙은 `openspec instructions`의 출력이 기준이다. 기억으로 쓰지 않는다. 출력의 `context`와 `rules`는 파일에 옮겨 쓰지 않는다.
- store를 쓰는 프로젝트는 모든 명령에 같은 `--store <id>`를 붙인다.

## 탐색

코드와 파일을 바꾸지 않는다.

1. `orai search '<질문>'`으로 결정·메시지·코드를 찾고 `orai read`로 원문을 읽는다.
2. 관련 스펙은 시나리오까지 전부 읽는다. `openspec list`로 겹치는 변경을 확인한다.
3. 확인할 수 있는 사실은 묻지 않는다. 사용자만 정할 수 있는 것을 한 번에 하나씩, 추천과 함께 묻는다.
4. 기록은 사용자가 요청하거나 동의한 뒤에 한다.

## 제안

계획 파일만 쓴다. 코드는 고치지 않는다.

1. 관련 스펙, 코드(`orai search '<질문>' --in code`), 기존 테스트, 겹치는 change를 확인한다. 범위·동작·호환성·수용 기준이 모호하면 먼저 묻는다.
2. `openspec new change <kebab-case-이름>`으로 만든다. 폴더를 손으로 만들지 않는다.
3. `openspec status --change <변경> --json`의 의존 순서대로 산출물마다 `openspec instructions <산출물> --change <변경> --json`을 실행하고 그 `instruction`과 `template`으로 쓴다.
   - proposal: 왜, 무엇을, 어느 기능(capability)을 바꾸는지. 기능 경로는 기존 것을 재사용한다.
   - specs: proposal의 기능마다 델타 파일 하나. 관찰 가능한 동작만 쓴다.
   - design: 여러 모듈, 새 의존성, 이전, 보안, 설계 선택이 있을 때만 쓴다.
   - tasks: `- [ ] 1.1 <작업과 확인 방법>`. 묶음마다 테스트와 문서를 포함한다. 보관은 체크박스로 넣지 않는다.
4. 동작이 바뀌지 않으면 `.openspec.yaml`에 `skip_specs: true`를 넣고 델타를 만들지 않는다. 검증을 통과시키려고 요구를 지어내지 않는다.
5. `openspec validate <변경> --type change --strict`를 통과시키고 계획을 보여 준 뒤 멈춘다. 구현은 새 요청이나 프로젝트 규칙의 승인 뒤에 시작한다.

### 델타 쓰기

| 절 | 규칙 |
|---|---|
| `## Purpose` | 새 기능에만, 맨 앞에 50자 이상으로 쓴다 |
| `## ADDED Requirements` | 새 요구. 본문에 SHALL이나 MUST, 시나리오 하나 이상 |
| `## MODIFIED Requirements` | 본 스펙의 요구 블록을 시나리오까지 전부 복사한 뒤 고친다. 제목은 그대로 둔다 |
| `## REMOVED Requirements` | `**Reason**`과 `**Migration**`을 쓴다. 기능의 마지막 요구를 없애면 `.openspec.yaml`에 `retire_capabilities: true` |
| `## RENAMED Requirements` | 이름만 바꾼다. ``- FROM: `### Requirement: 옛 이름` ``, ``- TO: `### Requirement: 새 이름` `` |

요구는 `### Requirement: <이름>`, 시나리오는 `#### Scenario: <이름>` 아래 `- **WHEN**`과 `- **THEN**`으로 쓴다.

## 계획 수정

코드는 고치지 않는다. `openspec status --change <변경> --json`의 기존 파일을 모두 읽고, 하나가 바뀌면 proposal·specs·design·tasks 전부를 서로 맞춘다. 요청받지 않은 범위 변경은 쓰기 전에 확인받는다. 이미 체크한 task가 새 계획과 맞지 않으면 체크를 풀고 알린다.

## 구현

1. `openspec instructions apply --change <변경> --json`을 실행한다. `state`가 `blocked`이면 멈춘다. `contextFiles`를 전부 읽고 `operationGuidance`를 따른다.
2. task마다 테스트 우선으로 구현하고, 그 동작이 실제로 끝났을 때만 `[x]`로 바꾼다.
3. 범위가 늘거나, 스펙의 동작을 줄이거나 미뤄야 하거나, 설계가 틀렸으면 멈추고 묻는다. 승인되면 계획 수정부터 한다.

### 테스트 우선

1. 이번에 만족시킬 시나리오를 정한다. 없거나 모호하면 구현하지 않는다.
2. 시나리오마다 테스트를 쓰고 실패하는 것을 본다. 테스트 이름이나 주석에 시나리오 이름을 남긴다.
3. 그 테스트가 통과할 만큼만 구현한 뒤 정리한다.
4. 프로젝트의 전체 검사 명령(AGENTS.md나 `mise tasks`)을 실행한다.

## 검증

`validate`는 형식 검사다. 구현이 맞는지는 따로 확인한다.

1. `openspec validate <변경> --type change --strict`
2. 델타의 시나리오마다 구현 위치와 통과한 테스트를 댄다. REMOVED는 동작이 사라졌는지, RENAMED는 동작이 그대로인지 확인한다.
3. design의 결정과 구현을 대조한다. 확인하지 못한 항목은 통과로 세지 않고 "미검증"으로 적는다.
4. 전체 검사와 `orai doctor`를 실행한다.

## 보관

`openspec archive`로만 한다. 본 스펙에 손으로 합치거나 change 폴더를 `mv`로 옮기지 않는다.

1. 최신 기준 브랜치에서 한다. `openspec instructions archive --change <변경> --json`의 안내를 따른다.
2. MODIFIED 요구마다 `openspec show <변경> --type change --diff`로 현재 본 스펙과 대조한다. 이 변경과 무관한 문장이 되돌아가면 현재 본 스펙을 기준으로 델타를 다시 쓴다.
3. 먼저 `--yes` 없이 `openspec archive <변경> --json`을 실행하고 `status`의 `code`로 정한다.
   - 오류 없음: 보관이 끝났다.
   - `archive_confirmation_required`: task가 모두 끝났고 스펙 갱신 확인만 남았다. `openspec archive <변경> --json --yes`로 다시 실행한다.
   - `archive_tasks_incomplete`: 열린 task를 보여 주고 멈춘다. 사용자가 그대로 보관하라고 할 때만 `--yes`를 붙인다.
   - 그 밖: 원인을 고친다.
4. `openspec validate --specs --strict`와 `git diff -- openspec/`로 결과를 확인한다.

- change 여러 개는 하나씩 보관하고, 매번 다음 change에 2번을 다시 한다.
- 보관하지 않고 본 스펙에만 반영(sync)하는 것은 사용자가 요청했을 때만 한다. 델타를 요구 단위로 합치고(델타 파일을 복사하지 않는다) `openspec validate --specs --strict`로 확인한다.

## 하지 않는 것

| 실수 | 올바른 처리 |
|---|---|
| 파일이 있으니 계획이 끝났다고 본다 (`status`는 존재만 본다) | 내용을 읽고 `validate --strict` |
| `validate` 통과를 구현 완료로 보고한다 | 시나리오별 테스트로 검증 |
| MODIFIED에 바뀐 줄만 쓴다 | 요구 블록 전체 |
| 같은 요구를 고친 다른 change 뒤에 그대로 보관한다 | 보관 2번. `validate`는 이 덮어쓰기를 잡지 못한다 |
| 처음부터 `--yes`를 붙이거나, 거부되자 `--no-validate`·`--skip-specs`로 넘긴다 | 원인을 고친다 |
| 구현 중 본 스펙을 먼저 고친다 | change의 델타를 고친다 |
| 안 한 검증의 task를 체크한다 | 열어 두고 "미검증"으로 보고 |
| OpenSpec 스킬을 설치하려고 `init`·`update`를 실행한다 | `orai setup` |

## 보고 형식

| 항목 | 내용 |
|---|---|
| 대상 | 커밋, 변경 이름(OpenSpec) |
| 시나리오별 검증 | 시나리오 → 테스트 이름 → 종류(fixture·fake, 실제 동작) → 결과 |
| 실행한 명령 | 전체 검사 명령과 결과 |
| 검증하지 못한 범위 | 실행하지 않은 시나리오와 그 이유 |

- fixture·fake 통과와 실제 동작을 섞어 쓰지 않는다.
- 실패한 검사를 성공으로 적지 않는다. 테스트를 건너뛰거나 지워서 통과시키지 않는다.

## 역할 사이에서

메시지 명령은 [../SKILL.md](../SKILL.md)에 있다.

| 때 | 보내는 것 |
|---|---|
| 작업을 맡길 때 | `orai send <역할> --kind todo`. 본문에 변경 이름(또는 스펙 경로)과 만족시킬 시나리오만 적는다. 상대 worktree에서 읽을 수 있게 계획이 커밋돼 있는지 먼저 확인한다 |
| 넘겨받았을 때 | `orai inbox <ID>`로 받고 변경의 원문을 읽는다. 지난 맥락은 `orai search '<변경 이름>' --in mail` |
| 기준이 모호할 때 | 구현 전에 `orai send <역할> --kind question` |
| 끝났을 때 | `orai reply <받은-ID>`로 위 보고 형식. 길면 `--file <경로>` |
