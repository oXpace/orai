# Proposal

## Why

QMD 2.8.3의 MCP `status`는 문서가 없는 묶음을 목록에서 뺀다. Orai는 이를 다른 index의 서버로 판정해, 빈 `openspec/specs`가 있는 새 프로젝트에서 `setup`, `init`, `recover`, `sync`, 자동 sync가 실패했다(2026-11-22 macOS에서 재현). 첫 문서를 넣어도 `sync`가 거부돼 풀리지 않았다.

## What Changes

- 서버 식별은 선언한 묶음 중 하나 이상이 이 프로젝트의 절대경로로 제공되고, 다른 경로로 제공되는 묶음이 없을 때 통과한다. 하나도 제공되지 않으면 계속 거부한다.
- 문서가 있는데 색인에 없는 묶음은 `shelf.collections` degraded로 알린다.

## Capabilities

### New Capabilities

- `shelf-engine`: shelf 엔진 서버의 식별과 진단

### Modified Capabilities

(없음)

## Impact

- 코드: `internal/shelf/client.go`, `internal/shelf/probe.go`.
- 다른 index의 서버를 건드리지 않는 계약은 유지한다(`TestIdentityStillRefusesAnotherIndex`).
