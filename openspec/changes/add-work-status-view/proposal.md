# Proposal

> 0.8 대상이다. 0.7.0에는 구현하지 않는다. 명령 이름과 JSON 형식은 설계에서 정한다.

## Why

작업 목록을 규칙으로 정리하면(`assign-work-with-changes`) 진행 상태가 역할 worktree마다 흩어진다. 병합 전의 체크는 프로젝트 폴더의 `openspec list`에 보이지 않는다.

## What Changes

- 역할 worktree마다 change의 진행률을 모아, 역할의 실행 여부와 받지 않은 메시지 수와 함께 보여 주는 읽기 전용 명령을 더한다.
- 하지 않는 것: 배정 저장, 임대, 다른 역할의 메일 본문 읽기.

## Capabilities

### New Capabilities

- `work-status`: 역할별 change 진행의 읽기 전용 현황

### Modified Capabilities

(없음)

## Impact

- 새 CLI 명령과 JSON 계약. `x-after`까지 보여 주려면 YAML 읽기 방법을 정해야 한다(새 의존성은 피한다).
