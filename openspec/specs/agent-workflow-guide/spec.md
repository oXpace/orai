# agent-workflow-guide Specification

## Purpose

setup이 생성하는 안내가 어느 역할에게나 같은 스펙·테스트 우선 순서와 보고 형식을 주되, 승인과 완료 기준은 프로젝트 규칙에 남긴다.

## Requirements

### Requirement: 스킬이 작업 순서 참조 문서를 갖는다
`orai setup`은 Orai 스킬에 `references/workflow.md`를 생성해야 한다(SHALL). 문서는 시나리오 확인, 실패 테스트 먼저, 최소 구현, 전체 검사, 스펙과 맞추기, 보고 순서를 담고, 승인권자와 완료 기준은 프로젝트 규칙과 역할 지침에 맡긴다.

#### Scenario: setup 뒤 참조 문서
- **WHEN** `orai setup --no-tools`를 실행한다
- **THEN** `.agents/skills/orai/references/workflow.md`가 생기고 `SKILL.md` 목차와 AGENTS 블록이 그 문서를 가리킨다

### Requirement: 보고는 검증 종류를 나눈다
안내의 보고 형식은 시나리오별로 테스트 이름과 종류(fixture·fake, 실제 동작)를 나누고, 실행한 명령과 결과, 검증하지 못한 범위를 따로 적게 해야 한다(SHALL).

#### Scenario: 보고 형식 표
- **WHEN** 생성된 `workflow.md`를 읽는다
- **THEN** "시나리오별 검증", "실행한 명령", "검증하지 못한 범위" 항목이 있다

### Requirement: 역할 사이 인계는 변경 이름으로 한다
안내는 작업을 맡길 때 메시지 본문에 변경 이름(또는 스펙 경로)과 만족시킬 시나리오만 적고 스펙 내용을 옮겨 쓰지 않게 해야 한다(SHALL).

#### Scenario: todo 인계 지침
- **WHEN** 생성된 `workflow.md`의 "역할 사이에서" 절을 읽는다
- **THEN** `todo` 본문에 변경 이름과 시나리오만 적으라는 지침이 있다
