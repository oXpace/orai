# Spec Delta

## RENAMED Requirements

- FROM: `### Requirement: setup은 프로젝트 범위로 한 번 init한다`
- TO: `### Requirement: setup은 스킬 없이 한 번 init한다`

## MODIFIED Requirements

### Requirement: setup은 스킬 없이 한 번 init한다
`[integrations.openspec]`이 있고 OpenSpec 2.0 이상이 설치돼 있으며 `openspec/changes`가 없으면, `orai setup`은 OpenSpec 스킬을 만들지 않도록 `openspec init --tools none`을 실행해야 한다(SHALL). `openspec/changes`가 있으면 다시 실행하지 않고, 이미 있는 OpenSpec 스킬은 지우거나 고치지 않는다.

#### Scenario: 처음 setup
- **GIVEN** OpenSpec 2.1.1이 설치되고 `openspec/changes`가 없다
- **WHEN** `orai setup`을 실행한다
- **THEN** `--tools none`을 포함한 init이 한 번 실행되고 `openspec-*` 스킬이 생기지 않는다
- **AND** 다시 `orai setup`을 실행하면 init이 실행되지 않는다

#### Scenario: 이미 설치된 OpenSpec 스킬
- **GIVEN** 프로젝트에 `.agents/skills/openspec-propose`가 있다
- **WHEN** `orai setup`을 실행한다
- **THEN** 그 스킬의 파일이 그대로 남는다

#### Scenario: 스킬 없이 준비된 프로젝트의 진단
- **GIVEN** `openspec/changes`가 있고 OpenSpec 스킬이 없다
- **WHEN** `orai doctor`를 실행한다
- **THEN** `openspec`이 healthy다
