# openspec-integration Specification

## Purpose

프로젝트가 선언하면 Orai가 OpenSpec을 프로젝트에 준비하고, 상태를 진단하며, 역할 세션의 텔레메트리를 기본으로 끈다.

## Requirements

### Requirement: 새 프로젝트는 OpenSpec 연동을 선언한다
`orai setup`이 새로 만드는 `orai.toml`은 `[integrations.openspec]`과 shelf 묶음 `openspec/specs`를 포함해야 한다(SHALL). 기존 `orai.toml`은 고치지 않는다.

#### Scenario: 빈 폴더에서 setup
- **WHEN** 빈 프로젝트에서 `orai setup --no-tools`를 실행한다
- **THEN** `orai.toml`에 `[integrations.openspec]`과 `specs = "openspec/specs"` 묶음이 있다

#### Scenario: 표가 없는 기존 프로젝트
- **GIVEN** `[integrations.openspec]`이 없는 `orai.toml`
- **WHEN** `orai setup`을 실행한다
- **THEN** `openspec/` 파일을 만들지 않고 doctor에 `openspec` 항목이 없다

### Requirement: setup은 프로젝트 범위로 한 번 init한다
`[integrations.openspec]`이 있고 OpenSpec 2.0 이상이 설치돼 있으며 프로젝트 스킬이 없으면, `orai setup`은 `openspec init --tools claude,codex --scope project`를 실행해야 한다(SHALL). 스킬이 이미 있으면 다시 실행하지 않는다.

#### Scenario: 처음 setup
- **GIVEN** OpenSpec 2.1.1이 설치되고 프로젝트 스킬이 없다
- **WHEN** `orai setup`을 실행한다
- **THEN** `--scope project`를 포함한 init이 한 번 실행된다
- **AND** 다시 `orai setup`을 실행하면 init이 실행되지 않는다

### Requirement: OpenSpec이 없어도 setup은 실패하지 않는다
OpenSpec이 설치돼 있지 않으면 `orai setup`은 OpenSpec 단계를 건너뛰고 설치 안내를 남겨야 하며(SHALL), `orai doctor`는 `openspec`을 blocked로 보고해야 한다(SHALL). 1.x는 unsupported로 보고하고 실행하지 않아야 한다(MUST).

#### Scenario: 설치되지 않았다
- **WHEN** OpenSpec이 PATH에 없는 상태로 `orai setup`을 실행한다
- **THEN** setup은 성공하고 설치 명령을 안내한다

#### Scenario: 1.x가 설치돼 있다
- **WHEN** OpenSpec 1.x가 있는 상태로 `orai setup`을 실행한다
- **THEN** init을 실행하지 않고 doctor가 `openspec`을 unsupported로 보고한다

### Requirement: config.yaml은 없을 때만 만든다
`[integrations.openspec]`이 있고 `openspec/config.yaml`이 없으면 setup은 테스트 우선 시작 규칙(`context`, `rules`, `operations`)으로 만들어야 한다(SHALL). 파일이 있으면 바이트 하나도 바꾸지 않아야 한다(MUST).

#### Scenario: 이미 있는 config.yaml
- **GIVEN** 사용자가 고친 `openspec/config.yaml`
- **WHEN** `orai setup`을 여러 번 실행한다
- **THEN** 파일 내용이 그대로다

### Requirement: 역할 세션의 텔레메트리는 기본으로 끈다
Orai가 시작하는 역할 세션과 Orai가 실행하는 OpenSpec 명령은 `OPENSPEC_TELEMETRY=0`을 받아야 한다(SHALL). `[integrations.openspec]`에 `telemetry = true`가 있으면 넣지 않는다.

#### Scenario: 기본
- **WHEN** 연동이 선언된 프로젝트에서 역할을 시작한다
- **THEN** 자식 환경에 `OPENSPEC_TELEMETRY=0`이 있다

#### Scenario: 프로젝트가 켰다
- **GIVEN** `telemetry = true`
- **WHEN** 역할을 시작한다
- **THEN** Orai는 `OPENSPEC_TELEMETRY`를 넣지 않고 사용자의 값을 그대로 둔다

### Requirement: deep 진단은 스펙을 검증한다
`orai doctor --deep`은 `openspec validate --specs` 결과를 `openspec.specs`로 보고해야 한다(SHALL). 실패한 스펙 이름을 보여 주고, 결과를 읽을 수 없으면 healthy로 보고하지 않아야 한다(MUST).

#### Scenario: 스펙 하나가 잘못됐다
- **GIVEN** Purpose가 없는 스펙
- **WHEN** `orai doctor --deep`을 실행한다
- **THEN** `openspec.specs`가 degraded이고 그 스펙 이름이 나온다
