# Proposal

## Why

OpenSpec 2.1.1의 `validate --specs --json`은 실패한 스펙이 있으면 보고서를 출력하고 종료 코드 1로 끝난다. Orai는 종료 코드만 보고 멈춰 `degraded`와 스펙 이름 대신 `blocked`를 냈다. 설치 안내의 Node 최소 버전(20.19)도 패키지의 engines(`>=22.20.0`)와 다르다. 표가 없는 프로젝트의 진단을 "항목이 없다"고 적은 스펙도 구현·테스트의 `not-configured`와 어긋난다.

## What Changes

- 종료 코드가 0이 아니어도 JSON 보고서를 먼저 읽는다. 실패한 스펙이 없는 실패 실행은 healthy로 보지 않는다.
- Node 안내를 22.20 이상으로 고친다.
- 스펙 문구를 `not-configured`로 맞춘다.

## Capabilities

### New Capabilities

(없음)

### Modified Capabilities

- `openspec-integration`: 실패한 검증의 보고, 선언이 없을 때의 진단 문구

## Impact

- 코드: `internal/openspec/openspec.go`, `internal/doctor/doctor.go`. 문서: `docs/compatibility.md`.
