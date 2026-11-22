# Spec Delta

## Purpose

검색 결과나 메시지 ID에서 원문으로 바로 가게 하되, 프로젝트 경계와 한 번만 수신 계약을 지킨다.

## ADDED Requirements

### Requirement: 프로젝트 안 파일의 줄 범위를 보여 준다
`orai read <경로>[:N[-M]]`는 프로젝트 안 파일을 줄 번호와 함께 보여 줘야 한다(SHALL). 범위가 없으면 처음 400줄, `:N`이면 N부터 80줄, `:N-M`이면 그 범위다. 경로는 현재 폴더, 프로젝트 폴더 순으로 찾고 shelf 결과의 `<묶음>/<경로>`도 받는다.

#### Scenario: 범위를 준다
- **WHEN** `orai read docs/operations.md:10-12`를 실행한다
- **THEN** 10, 11, 12번 줄이 줄 번호와 함께 나온다

### Requirement: 프로젝트 밖은 읽지 않는다
`orai read`는 프로젝트 루트 밖으로 해석되는 경로를 거부해야 한다(MUST).

#### Scenario: 상위 폴더를 가리킨다
- **WHEN** `orai read ../other/secret.txt`를 실행한다
- **THEN** 내용 없이 오류로 끝난다

### Requirement: 받지 않은 메시지는 보여 주지 않는다
메시지 ID를 주면 `orai read`는 이미 받았거나 보낸 메시지의 머리말과 본문만 보여 줘야 한다(SHALL). 받지 않은 메시지는 상태를 바꾸지 않고 `orai inbox <ID>`를 안내하며 거부해야 한다(MUST).

#### Scenario: 받지 않은 ID
- **GIVEN** `inbox/new`에 있는 메시지 ID
- **WHEN** `orai read <ID>`를 실행한다
- **THEN** 본문 없이 `orai inbox <ID>` 안내와 함께 거부된다
- **AND** 메시지는 `inbox/new`에 그대로 있고 receipt가 생기지 않는다
