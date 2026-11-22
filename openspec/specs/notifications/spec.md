# notifications Specification

## Purpose

새 메시지가 왔다는 사실만 역할 세션에 알리고, 받기와 처리는 역할이 명시적으로 하게 해 알림·적재·수신·완료를 서로 다른 상태로 둔다.

## Requirements

### Requirement: 알림은 메시지 ID만 담는다
알림 문구는 `오라이: 새 메시지` 줄과 `IDs: <id,…>` 줄로 고정해야 한다(SHALL). 메시지 본문, 보낸 사람, 제목을 담지 않는다(MUST).

#### Scenario: 메시지 하나가 도착한다
- **GIVEN** 알림이 준비된 역할 세션
- **WHEN** 그 역할에게 메시지가 배달된다
- **THEN** 세션이 받는 알림은 `오라이: 새 메시지\nIDs: <그 ID>`이고 본문이 없다

### Requirement: 알림은 메시지를 소비하지 않는다
notifier와 채널은 `inbox/new` 목록만 읽어야 하며 메시지를 옮기거나 receipt를 쓰지 않아야 한다(MUST). 수신과 receipt는 역할이 받기 명령을 실행할 때만 생긴다.

#### Scenario: 알림 뒤 메일함
- **WHEN** 메시지 알림이 전달된다
- **THEN** 그 메시지는 여전히 `inbox/new`에 있고 drained receipt가 없다

### Requirement: 같은 연결에서 같은 ID를 다시 알리지 않는다
한 연결 동안 이미 알린 ID는 다시 알리지 않아야 한다(SHALL). 전달이 실패한 ID는 다음 확인 주기에 다시 시도하고, 재연결하면 아직 `inbox/new`에 남은 ID를 다시 알린다.

#### Scenario: 두 번째 메시지
- **GIVEN** ID A를 이미 알렸다
- **WHEN** ID B가 도착한다
- **THEN** 다음 알림은 B만 담는다

#### Scenario: 전달 실패
- **GIVEN** 알림 전달이 한 번 실패했다
- **WHEN** 다음 확인 주기가 온다
- **THEN** 같은 ID를 다시 전달하고 메시지는 메일함에 그대로다

### Requirement: 준비되기 전에는 알리지 않는다
Codex 역할은 같은 nonce로 신원 캡처가 끝난 뒤에, Claude 역할은 채널 initialize와 `channel_ready` 호출 뒤에만 알림을 보내야 한다(SHALL). 그 전에 도착한 메시지는 준비된 뒤 알린다.

#### Scenario: 캡처 전에 메시지가 왔다
- **GIVEN** Codex 역할의 신원 캡처가 아직 끝나지 않았다
- **WHEN** 메시지가 도착한다
- **THEN** 알림을 보내지 않고, 캡처가 끝난 뒤 그 ID를 알린다
