## MODIFIED Requirements

### Requirement: 색인 지연을 알린다
QMD를 쓰는 프로젝트에서 마지막 shelf sync 표식보다 새 문서가 있으면 `orai search`는 바뀐 문서 수를 `note:`로 알려야 한다(SHALL). 색인이 없는 내장 검색은 이 알림을 내지 않는다. 코드 그래프가 뒤처졌을 때도 알린다.

#### Scenario: sync 뒤에 문서를 고쳤다
- **GIVEN** QMD를 쓰고 마지막 sync 뒤에 문서 하나를 고쳤다
- **WHEN** `orai search`를 실행한다
- **THEN** `note: 1 document(s) changed since the last sync`가 나온다

### Requirement: shelf가 답하지 않으면 키워드로 훑고 실패로 보고한다
QMD를 쓰는 프로젝트에서 서버가 없거나, 꺼졌거나, 다른 프로젝트 것이거나, 실패하면 `orai search`는 선언한 묶음의 파일을 키워드로 훑은 결과를 주고, 상태 줄에 이유와 고칠 명령을 적고, 종료 코드 1로 끝나야 한다(SHALL). 내장 검색을 쓰는 프로젝트에서 키워드 결과는 정상이며 종료 코드 0이어야 한다(MUST).

#### Scenario: shelf를 만들지 않았다
- **GIVEN** `engine = "qmd"`이지만 `orai shelf init`을 하지 않았다
- **WHEN** `orai search <낱말>`을 실행한다
- **THEN** 상태 줄이 `docs: keyword scan of the files`로 시작하고 `orai shelf init`을 안내한다
- **AND** 키워드 결과가 출력되고 종료 코드는 1이다

#### Scenario: 내장 검색
- **GIVEN** `engine`과 QMD 색인이 없다
- **WHEN** `orai search <낱말>`을 실행한다
- **THEN** 상태 줄이 `docs: built-in keyword search`로 시작하고 종료 코드는 0이다
