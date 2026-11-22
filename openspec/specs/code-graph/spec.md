# code-graph Specification

## Purpose

각 역할이 자기 checkout의 코드 구조를 보게 하고, 그래프의 생성·갱신·세션 연결을 Orai가 맡는다.

## Requirements

### Requirement: checkout마다 그래프를 만든다
`orai code init`과 `orai setup`은 CodeGraph가 설치돼 있으면 프로젝트 폴더와 존재하는 역할 worktree마다 그래프를 만들어야 한다(SHALL). `orai code sync`는 만들어진 그래프를 모두 갱신한다. Orai는 `codegraph install`을 실행하거나 전역 에이전트 설정을 바꾸지 않아야 한다(MUST).

#### Scenario: 역할 worktree가 있다
- **GIVEN** 역할 worktree 하나가 있는 프로젝트
- **WHEN** `orai code init`을 실행한다
- **THEN** 프로젝트 폴더와 그 worktree에 각각 `.codegraph/`가 생긴다

### Requirement: 그래프가 있는 역할에만 MCP를 연결한다
역할 세션을 시작할 때 그 역할의 작업 폴더에 그래프가 있으면 MCP 서버 `codegraph`를 그 작업 폴더 경로로 주입해야 한다(SHALL). 그래프가 없으면 주입하지 않아야 한다(MUST).

#### Scenario: 작업 폴더에 그래프가 있다
- **WHEN** 그래프가 있는 worktree의 역할을 시작한다
- **THEN** provider 인자에 `codegraph serve --mcp --path <그 worktree>`가 들어간다

#### Scenario: 작업 폴더에 그래프가 없다
- **WHEN** 그래프가 없는 역할을 시작한다
- **THEN** provider 인자에 `codegraph` MCP가 없다

### Requirement: 검색은 가장 가까운 그래프를 쓴다
`orai search --in code`는 실행 위치에서 가장 가까운 그래프를 써야 한다(SHALL). 그래프가 없으면 `code: FAILED`와 `orai code init`을 안내한다.

#### Scenario: worktree 안에서 찾는다
- **GIVEN** 프로젝트 폴더와 역할 worktree 모두에 그래프가 있다
- **WHEN** worktree 안에서 `orai search <심볼> --in code`를 실행한다
- **THEN** worktree의 그래프에 질의한다

### Requirement: 빠진 그래프를 진단한다
`orai doctor`는 그래프가 없는 역할 worktree를 `codegraph.worktrees` degraded로 보고하고 `orai code init`을 안내해야 한다(SHALL).

#### Scenario: 새 worktree를 만들었다
- **GIVEN** `git worktree add`로 역할 폴더를 새로 만들었다
- **WHEN** `orai doctor`를 실행한다
- **THEN** `codegraph.worktrees`가 그 폴더를 빠진 곳으로 알린다
