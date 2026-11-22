# Tasks

## 1. 수신을 막는 버그 (P1)

- [x] 1.1 깨진 파일로 목록이 멈추는 것을 테스트로 재현하고 실패를 확인한다
- [x] 1.2 목록 건너뛰기와 dlq 격리: `TestUnreadableMessageIsDeadLetteredWithoutBlockingTheInbox`, `TestReadingAnUnreadableMessageByIDDeadLettersIt` 통과로 확인한다

## 2. receipt 실패 시 유실 (P2)

- [x] 2.1 `TestAFailedReceiptStillHandsOverTheMessage` 통과로 확인한다
- [x] 2.2 리뷰 수정: 중간에 멈춘 dlq 이동 마무리 `TestAnInterruptedDeadLetterMoveIsFinished` 통과, 디렉터리 fsync EINVAL·ENOTSUP 허용(504801c), AMQ 교차 테스트의 todo 답장 기대값(eae8236)

## 3. 후순위 항목 (같은 커밋에 포함)

- [x] 3.1 알림 경로 파싱 제거: `TestPendingReadsNamesOnly` 통과로 확인한다
- [x] 3.2 todo 답장 kind: `TestReplyToTodoIsStatus` 통과로 확인한다
- [x] 3.3 디렉터리 fsync: 코드 검토로 확인. 전용 테스트는 `close-test-gaps`로 넘긴다

## 4. 병합

- [x] 4.1 PR #3을 통합 PR #5로 병합하고 trunk에서 검사 통과를 확인한다(2026-11-22 trunk 4d2886c)

## Workflow follow-up

- 실제 AMQ와의 교차 확인(특히 dlq 파일)은 하지 않기로 했다. 결정을 바꾸면 별도 변경으로 만든다.
