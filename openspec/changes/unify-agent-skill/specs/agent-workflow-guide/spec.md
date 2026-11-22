# Spec Delta

## ADDED Requirements

### Requirement: 스킬 하나가 OpenSpec 전 단계를 안내한다
Orai 스킬은 다른 스킬 없이 OpenSpec의 탐색, 제안, 계획 수정, 구현, 검증, 보관을 수행할 명령과 금지를 담아야 한다(SHALL).

#### Scenario: 단계 요청이 Orai 스킬로 온다
- **WHEN** 생성된 `SKILL.md`의 description을 읽는다
- **THEN** propose, apply, verify, sync, archive가 들어 있다

#### Scenario: 단계별 절
- **WHEN** 생성된 `workflow.md`를 읽는다
- **THEN** 탐색, 제안, 계획 수정, 구현, 검증, 보관 절이 있고 `openspec-*` 스킬을 읽으라는 지시가 없다

#### Scenario: 안전한 보관
- **WHEN** 생성된 `workflow.md`의 보관 절을 읽는다
- **THEN** 현재 스펙과의 diff 확인과 `--yes` 없는 첫 실행이 `--yes` 실행보다 앞에 있다

### Requirement: 옮겨 온 내용의 고지를 함께 배포한다
`orai setup`은 OpenSpec에서 옮겨 온 내용의 저작권과 MIT 허가 문구를 Orai 스킬과 함께 생성해야 한다(SHALL).

#### Scenario: setup 뒤 고지
- **WHEN** `orai setup --no-tools`를 실행한다
- **THEN** `.agents/skills/orai/references/openspec-license.txt`에 저작권자와 MIT 허가 문구가 있다
