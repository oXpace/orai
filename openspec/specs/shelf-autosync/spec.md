# shelf-autosync Specification

## Purpose

역할 세션이 떠 있는 동안 문서 변경이 사람이나 에이전트의 선택 없이 검색 색인에 반영되게 한다.

## Requirements

### Requirement: 세션 중 문서 변경을 sync한다
역할 세션이 떠 있는 동안 Orai는 shelf 묶음 폴더의 문서가 바뀌고 3초간 더 바뀌지 않으면 `orai shelf sync`와 같은 동작을 실행해야 한다(SHALL). 꺼진 서버는 켜고, 출력은 터미널이 아니라 `.orai/shelf/autosync.log`에 남긴다. 묶음 밖 파일의 변경에는 반응하지 않는다.

#### Scenario: 문서를 고쳤다
- **GIVEN** shelf가 만들어진 프로젝트에서 역할 세션이 떠 있다
- **WHEN** 묶음 폴더의 문서를 고치고 3초가 지난다
- **THEN** sync가 한 번 실행되고 `.orai/shelf/autosync.log`에 기록된다

#### Scenario: 묶음 밖 파일을 고쳤다
- **WHEN** 묶음에 속하지 않은 파일만 고친다
- **THEN** sync가 실행되지 않는다

### Requirement: 만들지 않은 shelf는 만들지 않는다
자동 sync는 `orai shelf init`을 하지 않은 프로젝트에서 색인을 만들지 않아야 한다(MUST).

#### Scenario: shelf가 없다
- **GIVEN** shelf를 init하지 않은 프로젝트에서 역할 세션이 떠 있다
- **WHEN** 묶음 폴더의 문서를 고친다
- **THEN** 색인 폴더가 생기지 않는다

### Requirement: 세션 시작 때 밀린 변경을 따라잡는다
역할 세션을 시작할 때 마지막 sync 표식보다 새 문서가 있으면 Orai는 sync를 한 번 실행해야 한다(SHALL). 다른 역할의 sync가 이미 반영한 변경이면 건너뛴다.

#### Scenario: 세션이 꺼진 동안 문서가 바뀌었다
- **GIVEN** 마지막 sync 뒤에 문서가 바뀌었다
- **WHEN** 역할 세션을 시작한다
- **THEN** 문서 변경 없이도 sync가 한 번 실행된다
