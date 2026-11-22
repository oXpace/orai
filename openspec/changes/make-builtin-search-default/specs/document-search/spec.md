## Purpose

설치와 서버 없이 프로젝트 문서를 찾게 하고, 의미 검색이 필요한 프로젝트만 QMD를 선택해 켜도록 검색 엔진을 정하고 진단한다.

## ADDED Requirements

### Requirement: 내장 검색이 표준이다
`[integrations.shelf]`에 `engine`이 없고 이 컴퓨터에 QMD 색인이 없거나 `engine = "builtin"`이면 Orai는 내장 키워드 검색을 써야 한다(SHALL). 이때 QMD를 실행하거나 설치를 요구하지 않고, MCP `shelf`를 주입하지 않아야 한다(MUST).

#### Scenario: 새 프로젝트
- **GIVEN** 문서가 있고 QMD가 설치돼 있으며 `engine`과 색인이 없다
- **WHEN** `orai setup`의 shelf 단계를 실행한다
- **THEN** `qmd`를 실행하지 않고 내장 검색을 보고한다
- **AND** 역할에 MCP `shelf`가 없고 doctor의 `shelf`는 healthy다

#### Scenario: 내장 검색으로 명시
- **GIVEN** QMD 색인이 있고 `engine = "builtin"`이다
- **WHEN** 검색한다
- **THEN** 서버에 묻지 않고 내장 검색으로 답한다

### Requirement: QMD는 켰을 때만 쓴다
`engine = "qmd"`이거나, `engine`이 없고 이 컴퓨터에 QMD 색인이 이미 있으면 Orai는 QMD를 써야 한다(SHALL). 내장 검색일 때 `stop`을 뺀 `orai shelf` 수명주기 명령은 켜는 법을 알리고 실패해야 한다(SHALL).

#### Scenario: QMD를 켰다
- **GIVEN** `engine = "qmd"`
- **WHEN** 색인 없이 검색한다
- **THEN** `orai shelf init`을 안내하는 대체 검색이다

#### Scenario: 0.6.0에서 쓰던 색인
- **GIVEN** `engine`이 없고 QMD 색인이 있다
- **WHEN** 엔진을 정한다
- **THEN** QMD를 계속 쓴다

#### Scenario: 켜지 않고 init
- **GIVEN** 내장 검색을 쓰는 프로젝트
- **WHEN** `orai shelf init`을 실행한다
- **THEN** `qmd`를 실행하지 않고 `engine = "qmd"`를 안내하며 실패한다

### Requirement: 내장 검색을 진단한다
내장 검색일 때 doctor는 문서 수를 보고하고, `--deep`에서 smoke가 있으면 기대 문서가 나오는지 확인해야 한다(SHALL). 프로젝트 MCP 설정에 `shelf`가 남아 있으면 degraded로 알린다.

#### Scenario: smoke
- **GIVEN** smoke의 기대 문서가 질의에 맞지 않는다
- **WHEN** `orai doctor --deep`을 실행한다
- **THEN** `shelf.search`가 degraded다

#### Scenario: 남은 MCP 등록
- **GIVEN** 내장 검색이고 `.mcp.json`에 `shelf`가 있다
- **WHEN** 진단한다
- **THEN** `shelf.registration`이 degraded다
