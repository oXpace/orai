## Purpose

역할 worktree에 흩어진 change의 진행을 메일 경계를 넘지 않고 한 화면에서 읽게 한다.

## ADDED Requirements

### Requirement: 역할별 진행을 읽기 전용으로 보여 준다
Orai는 역할 worktree마다 진행 중인 change와 task 진행률, 역할의 실행 여부, 받지 않은 메시지 수를 보여 주는 명령을 제공해야 한다(SHALL). 이 명령은 상태를 바꾸거나 다른 역할의 메시지 본문을 읽지 않아야 한다(MUST).

#### Scenario: 병합 전 진행
- **GIVEN** 역할 worktree에서만 task 하나를 체크했다
- **WHEN** 프로젝트 폴더에서 현황 명령을 실행한다
- **THEN** 그 역할의 진행률에 체크한 task가 들어 있다
