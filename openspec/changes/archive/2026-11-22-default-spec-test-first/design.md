# Design

## Context

OpenSpec 2.x의 기본 init 범위는 전역이고 텔레메트리가 켜져 있다. `openspec update`는 스킬 폴더를 통째로 다시 쓴다.

## Goals / Non-Goals

**Goals:**
- 새 프로젝트가 별도 작업 없이 스펙 계층과 테스트 우선 안내를 갖는다.
- OpenSpec 설정과 규칙은 프로젝트가 소유한다.

**Non-Goals:**
- 테스트 없이 구현하는 것을 막는 hook 게이트(사용자 결정으로 제외).
- OpenSpec을 설치·업그레이드하는 것.
- OpenSpec 없는 프로젝트에 다른 스펙 형식을 강제하는 것.

## Decisions

- **연동 표로 선언.** `[integrations.openspec]`이 있을 때만 setup·doctor가 다룬다. 지우면 진단에서 빠진다. 대안(항상 켬)은 OpenSpec을 쓰지 않는 프로젝트를 degraded로 만든다.
- **`--scope project` 항상 명시.** 전역 스킬 설치는 다른 프로젝트를 바꾼다(AGENTS 원칙 7).
- **config.yaml은 한 번만 쓴다.** OpenSpec이 `context`·`rules`·`operations`를 모든 지시에 넣으므로 테스트 우선 규칙을 여기 두면 OpenSpec 스킬을 고치지 않아도 된다. 이후 Orai는 다시 쓰지 않는다.
- **텔레메트리 기본 끔.** Orai가 띄운 프로세스에만 적용하고 사용자의 직접 실행은 사용자 설정을 따른다.
- **안내는 스킬 참조 문서.** 승인권자와 완료 기준은 프로젝트 규칙이 정하고, Orai는 순서와 보고 형식만 준다.

## Risks / Trade-offs

- [OpenSpec 버전 변화로 init 인자나 스킬 경로가 바뀜] → doctor가 버전과 스킬 유무를 확인하고, 1.x는 unsupported로 실행하지 않는다.
- [PR #1과 충돌] → #1 병합 뒤 다시 쌓는다.
