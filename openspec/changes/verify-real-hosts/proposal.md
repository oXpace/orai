# Proposal

## Why

Codex↔Claude 실제 흐름은 macOS에서 실사용으로 확인됐지만, 2026-11-22에 병합한 통합 검색·코드 그래프 운영·OpenSpec 연동은 fake·fixture 테스트와 Linux 클라우드에서만 확인했다. Linux는 실제 provider 실행이 미검증이다. `mise.toml`이 linux-x64·arm64를 지원 대상으로 적고 있으므로, 실제 호스트에서 확인하거나 지원 범위를 낮춰 적어야 한다.

## What Changes

- 사용자 Mac에서 병합한 기능의 실제 동작을 확인한다(보관한 change에서 넘어온 항목).
- 격리된 파일럿 저장소에서 운영 안내의 "실제 파일럿" 절차를 Linux에서 수행한다: 요청·회신과 알림, 종료 후 같은 대화로 재개, 중복 실행 거부, SessionStart hook 전제.
- shelf(QMD CPU fallback), CodeGraph, OpenSpec의 Linux 동작과 재부팅 뒤 복구를 확인한다.
- 결과를 `docs/history.md` 검증 기록과 `docs/compatibility.md`, README 상태 표에 반영한다. 확인하지 못한 항목은 미검증으로 남긴다.
- 코드 동작은 바꾸지 않는다. 확인 중 버그가 나오면 별도 변경으로 만든다.

## Capabilities

### New Capabilities

(없음. 검증과 문서 작업이라 `skip_specs: true`.)

### Modified Capabilities

(없음)

## Impact

- 문서: `docs/history.md`, `docs/compatibility.md`, `README.md`, 필요하면 `docs/operations.md`.
- 필요한 것: 사용자 Mac, Codex·Claude 로그인이 된 Linux 호스트(사용자 준비).
