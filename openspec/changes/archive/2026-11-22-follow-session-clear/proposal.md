# Proposal

## Why

역할 세션에서 `/clear`를 하면 provider의 세션 ID가 바뀌지만 Orai는 이전 ID를 유지한다. SessionStart hook의 matcher에 `clear`가 없고, `_capture`는 기록과 다른 ID를 거부한다. 다음 실행은 clear 이전 대화를 재개하고, Codex 알림은 이전 thread로 간다.

## What Changes

- SessionStart matcher에 `clear`를 넣는다.
- 같은 실행에서 확인된 `source=clear`에 한해 세션 ID와 transcript를 함께 교체하고 이전 상태를 `history/`에 남긴다.
- clear 뒤에는 역할 지침 확인과 `orai inbox`를 안내한다.
- hook 정의가 바뀌므로 Codex는 hook을 다시 신뢰해야 한다.

범위 밖: Codex `/new`, `/fork`, 세션 안 `/resume`, Claude `/branch`.

## Capabilities

### Modified Capabilities

- `role-sessions`: 새 대화의 시작 조건, 신원 캡처, clear 뒤 복원 문맥

## Impact

- `internal/providers`(matcher), `internal/runtime`(캡처, 문맥)
- architecture, operations, compatibility, history
