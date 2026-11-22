# Design

## Context

OpenSpec 2.1.1에는 스킬 생성만 끄는 `init --tools none`이 있고, 이 상태에서도 `config.yaml`의 `context`·`rules`·`operations`가 `openspec instructions`로 나온다. 산출물 형식은 CLI가 주므로 스킬에는 CLI가 주지 않는 절차와 금지만 있으면 된다.

## Goals / Non-Goals

**Goals:**
- 스킬 하나, 작업 흐름 문서 하나로 OpenSpec 전 단계를 수행한다.
- 메시지 알림마다 읽히는 `SKILL.md`는 커지지 않는다.

**Non-Goals:**
- OpenSpec CLI나 스키마를 복제하는 것.
- 사용자나 프로젝트가 이미 설치한 스킬을 지우는 것.
- 흐름을 강제하는 hook.

## Decisions

- **`init --tools none`.** 스킬을 만든 뒤 지우는 방법은 사용자 파일을 지우게 되고 `openspec update`가 되살린다. `none` 뒤의 `openspec update`는 스킬을 만들지 않는다.
- **준비 판정은 `openspec/changes`.** 스킬 파일은 더 이상 없고 `config.yaml`과 `openspec/specs`는 setup이 init보다 먼저 만든다.
- **`workflow.md`에 흡수.** 단계별 파일 6개로 나누면 다단 구조가 그대로이고, `SKILL.md`에 넣으면 알림마다 읽는 양이 세 배 가까이 된다.
- **형식은 `openspec instructions`에 맡긴다.** 스킬 96KB를 그대로 옮기면 CLI와 중복되고 버전마다 어긋난다.
- **보관은 CLI로만.** 원본 archive 스킬은 에이전트가 합치고 `mv`한다. CLI는 빠진 시나리오를 거부하고 실패하면 파일을 바꾸지 않는다.
- **`--yes`는 두 번째 실행에서만.** 스펙이 바뀌는 보관은 비대화형에서 항상 `--yes`가 필요하지만 그 플래그는 열린 task도 통과시킨다. 먼저 플래그 없이 실행해 `code`로 구분한다.
- **낡은 MODIFIED는 보관 전 diff로 잡는다.** 두 change가 같은 요구를 고치면 나중 것이 앞 것을 되돌려도 `validate --strict`와 `archive`가 통과한다.
- **MIT 고지 동봉.** `references/openspec-license.txt`에 저작권과 허가 문구를 넣는다.

## Risks / Trade-offs

- [OpenSpec 갱신으로 절차가 어긋남] → 기준 버전을 compatibility에 적고, 버전을 올릴 때 보관 코드와 `--tools none`을 실제 CLI로 다시 확인한다.
- [전역 `openspec-*` 스킬이 함께 보임] → 스킬에 우선할 절차를 적고, 전역 제거는 사용자가 정한다.
- [CLI 출력이 `/openspec-propose` 같은 없는 스킬을 권함] → 스킬 description이 같은 낱말로 트리거된다.
