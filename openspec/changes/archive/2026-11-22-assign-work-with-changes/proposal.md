# Proposal

## Why

배포 계획 4단계의 작업 목록(배정·임대·의존성)을 새 기능으로 만들면 다른 역할의 메일함을 읽어야 해서 지금의 경계를 깬다. 2026-11-22 검토에서 OpenSpec change와 메일 사용 규칙만으로 정리하는 A안을 제안했고, 사용자가 "좋아 그렇게 가보자"고 정했다. 메일 검색은 이름을 하이픈 단위로 나눠 맞추므로 change 이름 검색만으로는 다른 change가 섞인다.

## What Changes

- 맡길 때 `--kind todo --thread change/<이름>`과 본문 첫 줄 `change: <이름> task: <tasks.md의 번호>`를 쓰고, 답장도 같은 줄로 시작한다.
- 메일 검색 결과는 thread가 `change/<이름>`인 메시지만 그 작업으로 본다.
- change 안의 순서는 task 순서, change 사이는 `.openspec.yaml`의 `x-after`로 적고 선행 change가 보관된 뒤 맡긴다.
- 누가 맡길 수 있는지는 스킬이 아니라 역할 지침 템플릿에 적는다.
- `add-role-task-board` 초안은 이 규칙으로 대체한다. 읽기 전용 현황 명령은 `add-work-status-view`(0.8)로 남긴다.
- 하지 않는 것: 임대, 의존성 강제, 새 명령.

## Capabilities

### New Capabilities

(없음)

### Modified Capabilities

- `agent-workflow-guide`: 역할 사이 인계에 thread, task 번호, 의존성 규칙을 더한다

## Impact

- `internal/scaffold/templates`(workflow, 역할 지침 셋)와 생성 파일, 배포 계획.
