# Proposal

## Why

`orai <역할>`은 실행할 때마다 hook 신뢰를 설명하는 긴 문장을 낸다. 문제가 없어도, Claude 역할이어도 Codex 안내가 나오고, 무엇으로 어떻게 실행되는지는 읽기 어렵다. Codex와 Claude 전체 화면은 시작 출력을 가리므로 그 자리의 경고는 읽히지도 않는다.

2026-11-22 사용자는 시작 세 줄과 종료 한 줄(B안)을 고르고 검색 엔진 표시는 빼라고 했으며, 상황과 상태를 나눠 `orai status <역할>` 항목표도 넣기로 했다.

## What Changes

- 시작할 때 프로젝트·역할·대화, provider·모델·effort, 작업 폴더·새 메시지 수를 세 줄로 낸다.
- 끝날 때 이어가는 명령을 한 줄로 낸다.
- 경고는 이번 실행에서 실제로 생긴 문제만, 종료 뒤 한 줄로 낸다.
- 실행 안내는 stderr로 나간다. `--dry-run` JSON은 그대로 stdout이다.
- `orai run`에 `--quiet`(`-q`)를 추가한다.
- `orai status`에 붙인 인자를 더는 무시하지 않는다. 없는 역할 이름은 종료 코드 2다.
- `orai status <역할>`로 그 역할의 상태를 항목표로 본다. 인자 없는 `orai status`의 JSON은 그대로다.

## Capabilities

### Modified Capabilities

- `role-sessions`: 실행과 종료 때의 안내 출력, 역할 상태 항목표

## Impact

- 코드: `internal/runtime`, `internal/cli`.
- 문서: operations.
- 새 의존성 없음. 신원 캡처, 재개 판정, 알림 동작은 바꾸지 않는다.
