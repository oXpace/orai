## ADDED Requirements

### Requirement: 기획, 설계, 검증의 기록 위치를 나눈다
안내는 스펙에 요구와 동작 계약만, change에 설계·기술 결정과 구현 단계만 적고, 실검증·재검증·테스트 보강·버그는 프로젝트의 이슈 트래커에 두게 해야 한다(SHALL). 생성되는 OpenSpec 시작 규칙은 검증과 테스트를 tasks의 항목으로 요구하지 않아야 한다(MUST).

#### Scenario: 작업 흐름과 AGENTS 블록
- **WHEN** 생성된 `workflow.md`와 AGENTS 블록을 읽는다
- **THEN** 스펙, change, 이슈 트래커에 각각 무엇을 적는지 나와 있다

#### Scenario: 새 프로젝트의 시작 규칙
- **WHEN** 생성된 `openspec/config.yaml`의 tasks 규칙을 읽는다
- **THEN** 설계와 구현 단계만 적고 검증은 이슈로 보내라는 규칙이 있다
