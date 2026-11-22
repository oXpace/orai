# message-commands Specification

## Purpose

역할과 사용자가 메시지를 받고, 보내고, 회신하는 CLI 명령의 이름, 입력, 신원, 출력 계약을 정한다.

## Requirements

### Requirement: 역할 세션의 보내는 사람은 그 역할로 고정된다
역할 세션 안에서 메시지 명령의 신원은 그 역할이어야 하며(SHALL), `--as`로 바꾸려 하면 거부해야 한다(MUST). 역할 세션 밖에서는 `--as user`가 있어야 한다. 환경의 역할 신원이 서로 맞지 않거나 형식이 틀리면 거부한다.

#### Scenario: 역할 세션에서 --as를 쓴다
- **GIVEN** `lead` 역할 세션
- **WHEN** `orai msg inbox --as user`를 실행한다
- **THEN** "cannot override" 오류와 종료 코드 1로 끝난다

#### Scenario: 세션 밖에서 --as 없이 쓴다
- **WHEN** 역할 세션 밖에서 `orai msg inbox`를 실행한다
- **THEN** `--as user`를 안내하며 종료 코드 1로 끝난다

#### Scenario: 환경 신원이 어긋난다
- **GIVEN** `ORAI_ROLE=lead`이고 `AM_ME=dev`다
- **WHEN** `orai msg inbox`를 실행한다
- **THEN** 신원이 불완전하다는 오류로 끝난다

### Requirement: 본문 입력
`orai send`와 `orai reply`(그리고 `orai msg send|reply`)는 본문을 위치 인자, `--body`, `--file`, 파이프된 표준 입력 가운데 하나로 받아야 한다(SHALL). 본문 없이 표준 입력이 대화형 터미널이면 입력을 읽기 전에 종료 코드 1로 거부해야 한다(MUST).

#### Scenario: 본문을 인자로 보낸다
- **WHEN** `orai send dev "확인 부탁"`을 실행한다
- **THEN** dev의 `inbox/new`에 본문이 "확인 부탁"인 메시지가 생긴다

#### Scenario: 대화형 터미널에서 본문이 없다
- **WHEN** 표준 입력이 터미널인 상태에서 본문 없이 `orai msg send dev`를 실행한다
- **THEN** 입력을 기다리지 않고 종료 코드 1로 끝나며, 오류가 `--body, --file, or piped stdin`을 안내한다
- **AND** 메시지가 보내지지 않는다

### Requirement: ID로 받으면 그 메시지만 받는다
`orai msg inbox <ID>`는 그 메시지만 `cur/`로 옮기고 receipt를 남겨야 한다(SHALL). 다른 대기 메시지는 그대로 둔다.

#### Scenario: 두 메시지 중 하나를 받는다
- **GIVEN** `inbox/new`에 A와 B
- **WHEN** `orai msg inbox A`를 실행한다
- **THEN** A만 반환되고 B는 `inbox/new`에 남는다

### Requirement: 이미 받은 메시지는 다시 처리하게 하지 않는다
이미 받은 메시지를 ID로 다시 받으면 본문 없이 머리말과 `"already_received": true`, 받은 시각만 출력해야 한다(SHALL). `--again`을 주면 본문을 다시 보여 준다.

#### Scenario: 알림이 늦게 도착했다
- **GIVEN** 전체 수신으로 이미 받은 메시지 A
- **WHEN** `orai msg inbox A`를 실행한다
- **THEN** 출력에 `"already_received": true`가 있고 본문이 없다

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
