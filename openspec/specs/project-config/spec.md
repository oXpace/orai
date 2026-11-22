# project-config Specification

## Purpose

`orai.toml`과 `orai.local.toml`을 읽고 검증해, 모든 명령이 같은 설정을 보거나 같은 이유로 멈추게 한다.

## Requirements

### Requirement: 현재 schema만 읽는다
Orai는 `orai.toml`의 `schema`가 현재 값(2)일 때만 설정을 읽어야 한다(SHALL). 다른 값이면 모든 명령이 종료 코드 2로 멈추고, `orai doctor`는 `project`를 blocked로 보고해야 한다(MUST). Orai는 어떤 경우에도 `orai.toml` 본문을 고치지 않아야 한다(MUST).

#### Scenario: schema 1 파일은 고칠 두 줄과 함께 거부된다
- **GIVEN** `schema = 1`과 `[integrations.wiki]`가 있는 `orai.toml`
- **WHEN** `orai status`를 실행한다
- **THEN** 종료 코드는 2이고, 오류에 "set `schema = 2` and rename `[integrations.wiki]` to `[integrations.shelf]`"가 들어 있다
- **AND** `orai.toml`의 바이트는 실행 전과 같다

#### Scenario: doctor는 schema 1을 blocked로 본다
- **GIVEN** schema 1 `orai.toml`
- **WHEN** `orai doctor`를 실행한다
- **THEN** 종료 코드는 0이 아니고 `✗ project` 줄에 같은 두 줄 수정이 나온다

#### Scenario: schema 2에서 예전 표 이름은 모르는 키다
- **GIVEN** `schema = 2`와 `[integrations.wiki]`가 있는 `orai.toml`
- **WHEN** 설정을 읽는다
- **THEN** "unknown key(s) wiki" 오류로 거부된다

#### Scenario: 로컬 파일보다 공유 파일의 schema를 먼저 본다
- **GIVEN** schema 1 `orai.toml`과 비어 있는 `orai.local.toml`
- **WHEN** 두 파일을 합쳐 읽는다
- **THEN** 오류는 로컬 파일이 아니라 schema 1 거부 문구다
