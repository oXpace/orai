# shelf-engine Specification

## Purpose

선택형 shelf 엔진(QMD)의 서버가 이 프로젝트의 것인지 안전하게 가리고, 색인이 문서와 어긋난 곳을 진단한다.

## Requirements

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
비어 있지 않은 문서가 있는데 서버의 status에 없는 묶음은 `orai doctor`가 `shelf.collections` degraded로 보고하고, `orai shelf sync`와 묶음을 더했을 때의 `orai shelf stop && orai shelf refresh`를 안내해야 한다(SHALL). `orai search`는 이때 서버의 부분 결과를 정상으로 내지 않고, 키워드 대체 검색과 그 안내로 답해야 한다(MUST). 엔진이 색인하지 않는 빈 파일만 있는 묶음은 빠진 것으로 보지 않는다.

#### Scenario: 색인 전의 새 묶음
- **GIVEN** 문서가 있는 묶음이 status에 없다
- **WHEN** 진단한다
- **THEN** `shelf.collections`가 degraded다

#### Scenario: 색인 전의 묶음을 검색한다
- **GIVEN** 문서가 있는 묶음이 status에 없다
- **WHEN** `orai search`를 실행한다
- **THEN** 그 묶음까지 훑은 키워드 결과와 고칠 명령이 나오고 종료 코드는 1이다

#### Scenario: 빈 파일만 있는 묶음
- **GIVEN** 공백뿐인 `.md`만 있는 묶음이 status에 없다
- **WHEN** 진단한다
- **THEN** 색인에서 빠진 묶음으로 보고하지 않는다
