# Proposal

## Why

2026-11-22에 보관한 change를 스펙과 맞춰 보니, 코드 검토로만 확인한 시나리오가 세 개 남았다. 스펙의 시나리오마다 실패를 본 테스트가 있어야 한다는 이 프로젝트의 테스트 우선 원칙에 맞추기 위해 전용 테스트를 더한다.

## What Changes

- `shelf-autosync` "세션 시작 때 밀린 변경을 따라잡는다": 표식보다 새 문서가 있으면 세션 시작 때 sync가 한 번 실행되는지 확인하는 테스트.
- `mailbox` "성공으로 보고한 배달과 이동은 디렉터리까지 flush한다": 디렉터리 sync를 바꿔 끼울 수 있게 해, sync 실패 시 배달이 성공을 보고하지 않고 EINVAL·ENOTSUP은 성공으로 보는지 확인하는 테스트.
- `message-commands` "본문 입력"의 위치 인자 본문 시나리오 테스트.
- 동작은 바꾸지 않는다. 테스트가 실패하면 버그로 보고 별도 change를 만든다.

## Capabilities

### New Capabilities

(없음. 테스트만 더하므로 `skip_specs: true`.)

### Modified Capabilities

(없음)

## Impact

- 테스트: `internal/shelf`, `internal/mail`·`internal/state`, `internal/cli`. 디렉터리 sync를 패키지 수준 함수 변수로 노출할 수 있다(AGENTS.md 개발 환경 규칙).
