# Proposal

## Why

실사용에서 에이전트가 shelf를 거의 쓰지 않았다. 검색이 MCP에만 있어 Codex는 경로와 제목만 받았고, 역할 세션 밖에서는 따로 등록해야 했으며, 문서를 고칠 때마다 sync·recover·refresh 가운데 무엇을 할지 골라야 했다. 코드 그래프도 프로젝트 폴더 하나에만 있어 역할 worktree의 코드와 어긋났다. 에이전트가 쓰는 입구를 어느 provider에서나 그대로 전달되는 CLI 다섯 동사로 모으고, 색인 유지는 Orai가 맡는다.

## What Changes

- `orai search`: 문서(shelf), 자기 메일함, 코드 그래프를 소스별로 찾아 상태 줄과 결과를 글로 출력한다. shelf 서버가 답하지 않으면 파일을 키워드로 훑고 이유와 고칠 명령을 적는다. 아무 상태도 바꾸지 않는다.
- `orai read`: 프로젝트 안 파일의 줄 범위, 또는 이미 받았거나 보낸 메시지를 보여 준다. 받지 않은 메시지는 거부한다.
- `orai inbox|send|reply`: `orai msg inbox|send|reply`와 같은 명령(같은 출력). 본문을 인자로 줄 수 있다. 신원 고정·대화형 입력 거부·한 번만 수신 계약은 기준 스펙 그대로다.
- 역할 세션 중 문서가 바뀌면 3초 debounce 뒤 shelf sync를 자동 실행한다. sync가 시각 표식을 남겨 검색이 색인 지연을 알린다.
- `orai code init|sync|check`: 프로젝트 폴더와 모든 역할 worktree에 CodeGraph 그래프를 만들고 갱신한다. 역할 세션에는 자기 작업 폴더에 그래프가 있을 때만 MCP 서버 `codegraph`를 주입한다.
- **BREAKING** `search`, `read`, `code`가 예약어가 되어 역할 이름으로 쓸 수 없다.
- AGENTS 블록, 스킬, 문서가 다섯 동사를 먼저 안내한다.

## Capabilities

### New Capabilities

- `agent-search`: 문서·메일·코드 통합 검색의 출력, 대체 검색, 종료 코드, 무변경 보장.
- `agent-read`: 파일 줄 범위와 메시지 보기, 프로젝트 경계, 미수신 거부.
- `shelf-autosync`: 역할 세션 중 문서 변경을 색인에 반영하는 방식.
- `code-graph`: checkout별 코드 그래프의 생성·갱신·세션 연결.

### Modified Capabilities

- `message-commands`: 짧은 명령 이름과 예약어를 더하고, 본문 입력에 위치 인자를 더한다.

## Impact

- 새 패키지: `internal/search`, `internal/lexical`, `internal/cli/harness.go`, `internal/shelf/{search,autosync}.go`, `internal/codegraph/operate.go`, `internal/mail/search.go`.
- 바뀌는 곳: `internal/runtime`(감시와 MCP 주입), `internal/scaffold/templates`(AGENTS 블록, 스킬), 문서 전반.
- 외부 의존성 추가 없음(`fsnotify` 기존).
- 구현: 브랜치 `claude/project-thread-n7h559`(PR #1 위에 쌓음), PR #2, 통합 PR #6으로 병합.
