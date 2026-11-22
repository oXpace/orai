# mailbox Specification

## Purpose

역할과 사용자 사이 메시지를 AMQ schema 1과 같은 디스크 형식으로 잃지 않고 한 번만 전달한다.

## Requirements

### Requirement: 깨진 메시지 하나가 수신을 막지 않는다
`inbox/new`의 파일 하나를 파싱할 수 없어도 목록, drain, 알림은 나머지 메시지를 처리해야 한다(SHALL). drain이나 그 ID의 read는 그 파일을 바이트 그대로 `dlq/new/`로 옮기고 결과의 `dead_lettered`에 적어야 한다(SHALL). 옮길 때 복사본을 먼저 flush한 뒤 원본을 지운다.

#### Scenario: 깨진 파일과 정상 메시지가 함께 있다
- **GIVEN** `inbox/new`에 정상 메시지 하나와 머리말이 깨진 `.md` 하나
- **WHEN** drain한다
- **THEN** 정상 메시지가 반환되고 깨진 파일 ID가 `dead_lettered`에 있다
- **AND** 깨진 파일은 원래 바이트 그대로 `dlq/new/`에 있고 `inbox/new`에는 없다

#### Scenario: 깨진 메시지를 ID로 읽는다
- **WHEN** 깨진 파일의 ID로 read한다
- **THEN** 그 파일이 `dlq/new/`로 옮겨지고 결과가 격리를 알린다

#### Scenario: 중간에 멈춘 격리를 마저 끝낸다
- **GIVEN** 깨진 파일이 `dlq/new/`에 링크됐지만 `inbox/new`에서 아직 지워지지 않았다
- **WHEN** 다시 drain한다
- **THEN** drain이 실패하지 않고 `inbox/new`의 사본을 지워 격리를 끝낸다
- **AND** 이름만 같고 다른 파일이 `dlq/new/`에 있으면 덮어쓰지 않는다

#### Scenario: peek는 깨진 파일을 건너뛴다
- **WHEN** `inbox --peek`로 목록을 본다
- **THEN** 정상 메시지만 나오고 오류로 끝나지 않는다

### Requirement: receipt 실패가 메시지를 잃지 않는다
메시지를 `cur/`로 옮긴 뒤 receipt 쓰기가 실패하면 drain은 그 메시지를 `receipt_error`와 함께 돌려줘야 한다(SHALL). 그 메시지를 다음에 ID로 읽을 때 receipt를 써야 한다(SHALL).

#### Scenario: receipt 폴더에 쓸 수 없다
- **GIVEN** receipt 쓰기가 실패하는 상태
- **WHEN** drain한다
- **THEN** 메시지 본문이 반환되고 `receipt_error`가 붙는다
- **AND** 쓰기가 회복된 뒤 그 ID를 read하면 drained receipt가 생긴다

### Requirement: 성공으로 보고한 배달과 이동은 디렉터리까지 flush한다
배달, `cur/` 이동, dlq 이동, 상태 파일 교체는 성공을 보고하기 전에 파일과 그것이 들어간 디렉터리를 모두 fsync해야 한다(MUST). 파일시스템이 디렉터리 fsync를 지원하지 않으면(EINVAL, ENOTSUP) 그 쓰기를 실패로 보고하지 않는다.

#### Scenario: 배달
- **WHEN** 메시지를 보낸다
- **THEN** `inbox/new` 게시 뒤 그 디렉터리의 sync가 성공한 다음에야 보내기 결과가 나온다

#### Scenario: 디렉터리 fsync를 지원하지 않는 마운트
- **GIVEN** 디렉터리 fsync가 EINVAL을 돌려주는 파일시스템
- **WHEN** 상태 파일을 교체한다
- **THEN** 교체는 성공으로 보고된다

### Requirement: 알림은 메시지를 파싱하지 않는다
대기 메시지 확인(알림 경로)은 `inbox/new`의 파일 이름에서 ID만 얻어야 하며 파일 내용을 읽지 않아야 한다(SHALL).

#### Scenario: 깨진 파일이 있어도 알림 ID가 나온다
- **GIVEN** `inbox/new`에 내용이 깨진 `<id>.md`
- **WHEN** 대기 메시지를 확인한다
- **THEN** 파일을 읽지 않고 그 ID가 목록에 있다

### Requirement: 답장 kind는 원본 kind를 따른다
답장은 원본의 thread를 잇고 refs에 원본 ID를 넣어야 한다(SHALL). question에는 answer, review_request에는 review_response, todo에는 status(우선순위 low) kind를 붙인다.

#### Scenario: todo에 답한다
- **GIVEN** kind가 todo인 받은 메시지
- **WHEN** 그 메시지에 reply한다
- **THEN** 답장의 kind는 status, 우선순위는 low다
