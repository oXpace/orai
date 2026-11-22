# Spec Delta

## ADDED Requirements

### Requirement: 짧은 명령은 msg 명령과 같다
`orai inbox|send|reply`는 같은 인자에 대해 `orai msg inbox|send|reply`와 같은 출력과 종료 코드를 내야 한다(SHALL).

#### Scenario: 두 이름의 출력이 같다
- **WHEN** 같은 메일함 상태에서 `orai inbox --peek`와 `orai msg inbox --peek`를 실행한다
- **THEN** 두 출력과 종료 코드가 같다

### Requirement: 에이전트 명령 이름은 역할 이름이 될 수 없다
`search`, `read`, `code`, `inbox`, `send`, `reply`는 예약어여야 하며 `orai.toml`의 역할 이름으로 쓰면 거부해야 한다(MUST).

#### Scenario: 예약어 역할
- **GIVEN** `[roles.search]`가 있는 `orai.toml`
- **WHEN** 설정을 읽는다
- **THEN** 예약어라는 오류로 거부된다

## MODIFIED Requirements

### Requirement: 본문 입력
`orai send`와 `orai reply`(그리고 `orai msg send|reply`)는 본문을 위치 인자, `--body`, `--file`, 파이프된 표준 입력 가운데 하나로 받아야 한다(SHALL). 본문 없이 표준 입력이 대화형 터미널이면 입력을 읽기 전에 종료 코드 1로 거부해야 한다(MUST).

#### Scenario: 본문을 인자로 보낸다
- **WHEN** `orai send dev "확인 부탁"`을 실행한다
- **THEN** dev의 `inbox/new`에 본문이 "확인 부탁"인 메시지가 생긴다

#### Scenario: 대화형 터미널에서 본문이 없다
- **WHEN** 표준 입력이 터미널인 상태에서 본문 없이 `orai msg send dev`를 실행한다
- **THEN** 입력을 기다리지 않고 종료 코드 1로 끝나며, 오류가 `--body, --file, or piped stdin`을 안내한다
- **AND** 메시지가 보내지지 않는다
