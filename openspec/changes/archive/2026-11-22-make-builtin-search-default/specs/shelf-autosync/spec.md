## MODIFIED Requirements

### Requirement: 만들지 않은 shelf는 만들지 않는다
자동 sync는 QMD를 쓰지 않거나 `orai shelf init`을 하지 않은 프로젝트에서 색인을 만들거나 갱신하지 않아야 한다(MUST).

#### Scenario: shelf가 없다
- **GIVEN** shelf를 init하지 않은 프로젝트에서 역할 세션이 떠 있다
- **WHEN** 묶음 폴더의 문서를 고친다
- **THEN** 색인 폴더가 생기지 않는다

#### Scenario: 내장 검색으로 껐다
- **GIVEN** 색인이 남아 있고 `engine = "builtin"`이다
- **WHEN** 역할 세션이 시작된다
- **THEN** sync가 실행되지 않는다
