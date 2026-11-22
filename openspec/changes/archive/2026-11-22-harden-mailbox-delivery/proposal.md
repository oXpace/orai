# Proposal

## Why

`inbox/new`에 파싱할 수 없는 파일이 하나라도 있으면 그 역할의 목록·수신·알림이 모두 멈춘다(임시 테스트로 재현). `cur/`로 옮긴 뒤 receipt 쓰기가 실패하면 본문을 돌려주지 않아 메시지가 사라진다. AMQ가 0.85.0 이후 같은 계열의 문제를 고쳤으므로, 디스크 형식은 그대로 두고 Orai 메일함에 해당하는 수정을 반영한다.

## What Changes

- 파싱할 수 없는 메시지는 목록에서 빠지고, drain이나 그 ID의 read가 원본 그대로 `dlq/new/`로 옮겨 결과의 `dead_lettered`에 적는다(P1).
- receipt 쓰기가 실패해도 drain은 메시지를 돌려주고 `receipt_error`를 붙이며, 다음 ID read가 receipt를 쓴다(P2, AMQ 0.133.0).
- 배달, `cur/` 이동, dlq 이동, 모든 상태 파일 교체가 디렉터리도 fsync한다(AMQ 0.137.0, 0.139.0). 디렉터리 fsync를 지원하지 않는 파일시스템(EINVAL, ENOTSUP)에서는 쓰기를 실패로 보고하지 않는다. (후순위였으나 같은 커밋에 들어감)
- 링크 뒤에 멈춘 dlq 이동은 다음 drain이 마저 끝낸다(리뷰에서 발견).
- 알림 경로(`Pending`)가 파일 이름에서 ID만 얻고 파싱하지 않는다. (후순위였으나 같은 커밋에 들어감)
- todo에 대한 답장은 status kind(우선순위 low)다(AMQ 0.124.0). (후순위였으나 같은 커밋에 들어감)
- 디스크 형식은 바꾸지 않는다. dlq 폴더는 AMQ 형식의 일부다.

## Capabilities

### New Capabilities

- `mailbox`: AMQ schema 1 호환 메일함의 배달, 수신, receipt, 격리, 내구성, 답장 kind.

### Modified Capabilities

(없음)

## Impact

- 코드: `internal/mail/queue.go`, `internal/state/state.go`(`SyncDir`).
- 문서: `docs/architecture.md` 메일함 절, `docs/history.md`.
- 호환: 실제 AMQ와의 교차 테스트는 사용자 결정으로 다시 돌리지 않는다. AMQ가 dlq 파일에 부가 정보를 기대하는지는 미확인.
- 구현: 브랜치 `claude/project-thread-lzhbcq`, PR #3, 리뷰 수정과 함께 통합 PR #5로 병합.
