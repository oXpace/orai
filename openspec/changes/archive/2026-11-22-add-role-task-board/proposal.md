# Proposal

> 규칙으로 대체(2026-11-22 사용자 결정). 구현하지 않았다. 배정과 의존성은 `assign-work-with-changes`의 규칙이, 읽기 전용 현황은 `add-work-status-view`(0.8)가 맡고, 임대는 범위에서 뺐다.

## Why

역할 사이 일은 지금 메시지(`todo`, `status`)로만 오간다. 누가 무엇을 맡았는지, 맡은 일이 아직 살아 있는지, 무엇이 무엇을 기다리는지를 메일함 기록에서 다시 짜 맞춰야 한다. 배포 계획 4단계(작업 목록: 역할 간 작업 배정·임대·의존성)로 이 상태를 한곳에 둔다.

## What Changes

- 메일함 위에 역할 간 작업 목록을 둔다: 배정, 임대(lease: 맡은 역할이 일정 시간 갱신하지 않으면 풀림), 작업 사이 의존성.
- 범위와 형식은 설계 합의 뒤 정한다.

## Open Questions

- 저장 위치와 형식: 메일함 안(AMQ와의 호환) 또는 `.orai/` 상태.
- OpenSpec 변경과의 관계: 작업 하나가 변경의 task 하나에 대응하는가.
- 임대 시간과 만료 뒤 처리, 사용자 승인이 필요한 전이.
- 이슈 트래커 정책은 소비 프로젝트 소유라는 경계와 어디서 나뉘는가.

## Capabilities

### New Capabilities

- `task-board`: 역할 간 작업 배정·임대·의존성 (설계 합의 뒤 스펙 작성)

### Modified Capabilities

- 미정 (`mailbox`, `message-commands`에 영향 가능)

## Impact

- 미정. `docs/release.md` 4단계의 진입 조건은 "설계 문서 합의 후".
