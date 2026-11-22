# Proposal

## Why

OpenSpec 스킬 6개(96KB, 전체 프로필은 12개 185KB)와 Orai 스킬이 같은 일을 나눠 안내해 에이전트가 여러 스킬을 오가고, 서로 다른 절차(예: 손으로 합치고 `mv`하는 보관과 `openspec archive`)를 따르게 된다. 연동이 아직 릴리스되지 않았으므로 0.7.0 전에 하나로 합치면 이전 경로가 필요 없다.

## What Changes

- `orai setup`은 OpenSpec을 스킬 없이(`--tools none`) init한다.
- Orai 스킬의 `references/workflow.md`가 OpenSpec의 탐색, 제안, 계획 수정, 구현, 검증, 보관과 Orai 명령 활용을 함께 안내한다.
- 옮겨 온 내용의 MIT 고지를 스킬과 함께 배포한다.
- 이 저장소도 `[integrations.openspec]`과 shelf 묶음 `specs`를 선언한다.
- 이미 설치된 `openspec-*` 스킬은 프로젝트·전역 모두 지우지 않는다.

## Capabilities

### New Capabilities

없음.

### Modified Capabilities

- `openspec-integration`: init이 스킬을 만들지 않고, 준비 여부를 `openspec/changes`로 판정한다.
- `agent-workflow-guide`: 스킬 하나가 OpenSpec 전 단계를 안내하고 라이선스 고지를 포함한다.

## Impact

- `internal/openspec`, `internal/scaffold`(템플릿 4개와 고지 1개), `internal/cli` 도움말
- 이 저장소의 `orai.toml`, 생성 스킬, AGENTS 블록
- README, architecture, operations, compatibility, history
