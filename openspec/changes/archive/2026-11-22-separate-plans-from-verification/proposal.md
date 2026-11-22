# Proposal

## Why

스펙은 기획(요구와 동작 계약)이고 change의 체크리스트는 설계와 기술 결정(ADR 성격), 그 구현 단계다. 검증, 재검증, 테스트 보강, 버그까지 함께 적으면 무엇을 만들기로 했는지가 검증 기록에 묻힌다(2026-11-22 사용자 규칙).

## What Changes

- `openspec/specs`는 요구와 동작 계약만, change는 설계와 기술 결정, 구현 단계만 담는다.
- 실검증, 재검증, 테스트 보강, 버그는 프로젝트의 이슈 트래커에 둔다. 이 저장소에서는 GitHub 이슈(oXpace/orai)다.
- 생성되는 작업 흐름, AGENTS 블록, 새 프로젝트의 OpenSpec 시작 규칙이 이 구분을 안내한다.
- 테스트 우선은 구현하는 방법으로 남는다. tasks의 항목으로 적지 않는다.

## Capabilities

### New Capabilities

(없음)

### Modified Capabilities

- `agent-workflow-guide`: 무엇을 어디에 기록하는지 구분한다

## Impact

- `internal/scaffold/templates`, 이 저장소의 `AGENTS.md`와 `openspec/config.yaml`, 진행 중인 change.
