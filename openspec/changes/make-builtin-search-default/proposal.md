# Proposal

## Why

QMD는 설치, 모델(약 2.6 GB), 서버 포트, 재부팅 뒤 복구까지 관리할 것이 많다. 2026-11-22 사용자는 "qmd 관리가 어려운 부분들이 많으니까 그건 앞으로 플러그인 기능으로 놓고" wikimap 방식 검색을 표준으로 두자고 했고, Go로 이식한 내장 검색(`internal/lexical`)으로 하자는 제안에 "그렇게 가자"고 답했다.

## What Changes

- **BREAKING**: `[integrations.shelf]`의 기본 엔진이 내장 키워드 검색이다. 서버, 색인, 설치가 없고 `orai search`는 이 결과를 정상(종료 코드 0)으로 낸다.
- QMD는 `engine = "qmd"`로 켜는 선택형이다. 켰을 때만 서버, 의미 검색, MCP `shelf`, 자동 sync가 동작한다.
- `engine`을 적지 않았어도 이 컴퓨터에 QMD 색인(`.orai/shelf`)이 이미 있으면 QMD를 계속 쓴다. 0.6.0에서 쓰던 의미 검색이 업그레이드로 꺼지지 않게 한다. `engine = "builtin"`으로 명시해서 끈다.
- `orai setup`은 QMD를 켜지 않은 프로젝트에서 `qmd`를 실행하거나 설치를 요구하지 않는다.

## Capabilities

### New Capabilities

- `document-search`: 문서 검색 엔진의 선택, 내장 검색의 준비와 진단

### Modified Capabilities

- `agent-search`: 대체 검색으로 실패 처리하는 것은 QMD를 켰을 때만이다
- `shelf-autosync`: QMD를 쓰는 프로젝트에서만 동작한다

## Impact

- 코드: `internal/config`, `internal/shelf`, `internal/search`, `internal/cli`, `internal/scaffold/templates`.
- 문서: README, architecture, operations, compatibility, history.
- 새 의존성 없음. `orai.toml` schema는 2 그대로다.
