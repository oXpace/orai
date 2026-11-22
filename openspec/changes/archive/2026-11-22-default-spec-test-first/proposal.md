# Proposal

## Why

`orai setup`만 하면 역할 세션이 스펙을 기준으로 시작하고 실패 테스트부터 쓰는 흐름을 따르게 하고 싶다. 업무 규칙은 소비 프로젝트 소유라는 경계 안에서, Orai는 스펙 도구(OpenSpec)의 준비와 진단, 그리고 역할 사이 인계·보고 형식만 제공한다.

## What Changes

- 새 `orai.toml`이 `[integrations.openspec]`과 shelf 묶음 `specs = "openspec/specs"`(설명 포함)를 선언한다. 기존 `orai.toml`은 고치지 않는다.
- `orai setup`: OpenSpec 2.0 이상이 있고 프로젝트 스킬이 없으면 `openspec init --tools claude,codex --scope project`를 한 번 실행한다. `openspec/config.yaml`이 없으면 테스트 우선 시작 규칙으로 만들고, 있으면 건드리지 않는다. OpenSpec이 없으면 설치 안내만 하고 실패하지 않는다.
- 역할 세션과 Orai가 실행하는 OpenSpec에 `OPENSPEC_TELEMETRY=0`을 넣는다. `telemetry = true`로 켤 수 있다.
- `orai doctor`: `openspec`(설치, 2.0 이상, 프로젝트 스킬), `--deep`에서 `openspec.specs`(`openspec validate --specs`).
- Orai 스킬에 `references/workflow.md`: 시나리오 확인 → 실패 테스트 → 최소 구현 → 전체 검사 → 스펙과 맞추기 → 검증 종류를 나눈 보고, 역할 사이에는 변경 이름으로 인계. AGENTS 블록과 manager·engineer·role 템플릿이 이를 가리킨다.
- 강제 게이트(hook)는 넣지 않는다.
- **BREAKING(조건부)** `[integrations.openspec]`을 넣은 `orai.toml`은 이 표를 모르는 이전 Orai가 거부한다.

## Capabilities

### New Capabilities

- `openspec-integration`: OpenSpec 연동의 선언, setup 동작, 텔레메트리, 진단.
- `agent-workflow-guide`: setup이 생성하는 스펙·테스트 우선 작업 안내와 보고 형식.

### Modified Capabilities

(없음)

## Impact

- 새 패키지 `internal/openspec`, 템플릿 `openspec-config.yaml`, `skill-workflow.md`.
- 바뀌는 곳: `internal/config`(표 파싱), `internal/setup`, `internal/runtime`(자식 환경), `internal/doctor`, `internal/scaffold` 템플릿, 문서.
- 외부 도구: OpenSpec 2.x(사용자 설치, 확인 버전 2.1.1). 전역 `~/.local/share/openspec/projects.json`은 OpenSpec 자신이 갱신하며 Orai는 읽거나 쓰지 않는다.
- 구현: 브랜치 `claude/project-thread-oligl4` 커밋 151daaa(패치 `openspec-tdd/`). PR #4, 통합 PR #8로 병합.
