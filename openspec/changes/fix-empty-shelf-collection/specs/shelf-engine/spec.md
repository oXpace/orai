## Purpose

선택형 shelf 엔진(QMD)의 서버가 이 프로젝트의 것인지 안전하게 가리고, 색인이 문서와 어긋난 곳을 진단한다.

## ADDED Requirements

### Requirement: 문서가 없는 묶음이 서버 식별을 막지 않는다
Orai는 선언한 묶음 중 하나 이상이 이 프로젝트의 절대경로로 제공되고 다른 경로로 제공되는 묶음이 없으면 그 서버를 이 프로젝트의 것으로 보아야 한다(SHALL). 하나도 제공되지 않거나 경로가 하나라도 다르면 거부하고 그 서버를 건드리지 않아야 한다(MUST).

#### Scenario: 빈 묶음
- **GIVEN** 문서가 있는 `docs`와 비어 있는 `specs` 묶음
- **WHEN** 서버의 status가 `docs`만 이 프로젝트 경로로 보고한다
- **THEN** 서버 식별이 통과한다

#### Scenario: 첫 문서를 sync하기 전
- **GIVEN** 비어 있던 묶음에 첫 문서가 생겼고 status에는 아직 없다
- **WHEN** 서버를 식별한다
- **THEN** 통과해서 `orai shelf sync`가 그 문서를 색인할 수 있다

#### Scenario: 다른 index
- **WHEN** 제공되는 묶음이 없거나, 하나라도 다른 경로를 제공한다
- **THEN** 서버를 거부한다

### Requirement: 색인에 없는 문서를 진단한다
문서가 있는데 서버의 status에 없는 묶음은 `orai doctor`가 `shelf.collections` degraded로 보고하고 `orai shelf sync`를 안내해야 한다(SHALL).

#### Scenario: 색인 전의 새 묶음
- **GIVEN** 문서가 있는 묶음이 status에 없다
- **WHEN** 진단한다
- **THEN** `shelf.collections`가 degraded다
