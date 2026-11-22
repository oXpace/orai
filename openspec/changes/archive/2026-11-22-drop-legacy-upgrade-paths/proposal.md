# Proposal

## Why

0.1~0.3에서 올라오는 경로(schema 1 읽기, `.orai/wiki` 색인 이전, 예전 명령 이름 안내, `pypi:` 고정 인식)가 코드와 문서에 남아 새 기능을 넣을 때마다 함께 고려해야 한다. 0.6.0이 그 경로를 모두 갖고 있으므로, 0.7.0에서 0.x 정책대로 호환을 끊고 0.3 이하는 0.6.0을 거쳐 올리게 한다.

## What Changes

- **BREAKING** `orai.toml`의 `schema = 1`(`[integrations.wiki]`)을 더 읽지 않는다. 모든 명령이 고칠 두 줄을 담은 오류로 거부하고 파일은 고치지 않는다. doctor의 "schema 1 사용" degraded 항목은 없어지고 `project` blocked가 된다.
- **BREAKING** shelf가 `.orai/wiki/` 색인을 `.orai/shelf/`로 옮기지 않는다.
- `orai inbox|send|reply|qmd|wiki|init`이 새 이름을 안내하던 표를 지운다. 이름은 예약어로 남는다(`inbox|send|reply`는 `unify-agent-harness-cli` 변경에서 다시 명령이 된다).
- setup의 고정 검사가 0.1.0의 `pypi:oXpace/orai`를 Orai 고정으로 보지 않는다.
- 추적되던 Python 시절 `__pycache__` 58개를 지운다(동작 변화 없음).
- 운영 안내의 0.1~0.4 마이그레이션 절을 "0.3 이하에서 올릴 때" 한 절로 줄이고 0.6.0 태그 문서를 가리킨다.
- 남기는 것: `--preset pm-staff` 별칭, 이전 관리 블록 표식의 접두어 교체, 같은 포트를 다른 이름으로 등록한 MCP 설정 진단.

## Capabilities

### New Capabilities

- `project-config`: `orai.toml`(공유)과 `orai.local.toml`(로컬)을 읽고 검증하는 규칙. 이 변경은 schema 판정 요구만 적는다.

### Modified Capabilities

(없음)

## Impact

- 코드: `internal/config`, `internal/runtime/diagnose.go`, `internal/cli`, `internal/shelf`(이전 경로), `internal/scaffold`(`misePinNote`).
- 문서: `README.md` 상태 표, `docs/architecture.md`, `docs/operations.md`, `docs/history.md`, `docs/release.md`.
- 사용자: 0.3 이하 프로젝트는 0.6.0을 먼저 거쳐야 한다.
- 구현: 브랜치 `claude/project-thread-tapp53`, PR #1 (병합).
