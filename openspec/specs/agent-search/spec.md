# agent-search Specification

## Purpose

에이전트가 어느 provider에서든 한 명령으로 프로젝트 문서, 자기 메일함, 코드 구조를 찾고, 결과를 믿어도 되는지 함께 알게 한다.

## Requirements

### Requirement: 소스별 상태와 결과를 글로 출력한다
`orai search <질의>`는 문서·메일·코드 소스마다 상태 줄 하나를 먼저 찍고, 그 소스의 결과와 결과를 여는 `orai read` 명령을 출력해야 한다(SHALL). 소스의 결과를 한 순위로 섞지 않아야 한다(MUST). 설정하지 않은 소스는 `not searched`로 표시하고 실패로 세지 않는다.

#### Scenario: 설정된 소스가 모두 답한다
- **GIVEN** shelf 서버와 코드 그래프가 준비되고 역할 세션 안이다
- **WHEN** `orai search <낱말>`을 실행한다
- **THEN** docs, mail, code 상태 줄이 각각 하나씩 나오고 종료 코드는 0이다

#### Scenario: 소스를 고른다
- **WHEN** `orai search <낱말> --in docs`를 실행한다
- **THEN** docs 상태 줄과 결과만 나온다

### Requirement: 검색은 상태를 바꾸지 않는다
`orai search`는 메시지를 받지 않고, shelf 서버를 켜지 않으며, 색인을 갱신하지 않아야 한다(MUST).

#### Scenario: 깨진 메시지가 메일 소스를 막지 않는다
- **GIVEN** 메일함에 파싱할 수 없는 메시지 파일이 있다
- **WHEN** `orai search <낱말> --in mail`을 실행한다
- **THEN** 그 파일을 건너뛰고 나머지 메시지에서 결과를 낸다

#### Scenario: 받지 않은 메시지는 개수만 알린다
- **GIVEN** 질의에 맞는 메시지가 `inbox/new`에 있다
- **WHEN** `orai search`를 실행한다
- **THEN** `note: N message(s) not received yet also match`가 나오고 본문은 나오지 않는다
- **AND** 그 메시지는 여전히 `inbox/new`에 있다

### Requirement: shelf가 답하지 않으면 키워드로 훑고 실패로 보고한다
shelf 서버가 없거나, 꺼졌거나, 다른 프로젝트 것이거나, 실패하면 `orai search`는 선언한 묶음의 파일을 키워드로 훑은 결과를 주고, 상태 줄에 이유와 고칠 명령을 적고, 종료 코드 1로 끝나야 한다(SHALL).

#### Scenario: shelf를 만들지 않았다
- **GIVEN** shelf 묶음은 선언했지만 `orai shelf init`을 하지 않았다
- **WHEN** `orai search <낱말>`을 실행한다
- **THEN** 상태 줄이 `docs: keyword scan of the files`로 시작하고 `orai shelf init`을 안내한다
- **AND** 키워드 결과가 출력되고 종료 코드는 1이다

### Requirement: 색인 지연을 알린다
마지막 shelf sync 표식보다 새 문서가 있으면 `orai search`는 바뀐 문서 수를 `note:`로 알려야 한다(SHALL). 코드 그래프가 뒤처졌을 때도 같다.

#### Scenario: sync 뒤에 문서를 고쳤다
- **GIVEN** 마지막 sync 뒤에 문서 하나를 고쳤다
- **WHEN** `orai search`를 실행한다
- **THEN** `note: 1 document(s) changed since the last sync`가 나온다

### Requirement: 역할 세션 밖에서는 요청할 때만 메일을 찾는다
역할 세션 밖에서 `orai search`는 `--as user`가 있을 때만 사용자 메일함을 찾아야 한다(SHALL). 역할 세션 안에서는 그 역할의 메일함만 찾는다.

#### Scenario: 세션 밖에서 --as 없이 찾는다
- **WHEN** 역할 세션 밖에서 `orai search <낱말>`을 실행한다
- **THEN** mail은 `not searched`로 표시된다

### Requirement: 키워드 대체 검색은 드문 낱말과 제목을 무겁게 매긴다
키워드 대체 검색은 Markdown 문서를 제목으로 절을 나누고, 낱말마다 묶음 안의 드문 정도(IDF)로 무게를 주며, 문서 제목·경로·절 제목에 맞으면 본문보다 무겁게 매겨야 한다(SHALL). 결과는 문서마다 가장 나은 절과 그 안의 가장 나은 줄을 가리킨다. 코드 블록 안의 `#` 줄은 제목으로 보지 않는다.

#### Scenario: 드문 낱말이 있는 문서가 앞선다
- **GIVEN** 흔한 낱말이 많은 긴 문서와, 질문의 드문 낱말이 있는 짧은 문서
- **WHEN** shelf 없이 `orai search`를 실행한다
- **THEN** 드문 낱말이 있는 문서가 먼저 나오고 그 절의 줄 번호를 가리킨다

#### Scenario: 제목에 맞는 문서
- **GIVEN** 같은 낱말이 한 문서에서는 제목에, 다른 문서에서는 본문에 있다
- **WHEN** 그 낱말로 찾는다
- **THEN** 제목에 맞는 문서가 앞선다

### Requirement: 짧은 질문은 모든 낱말이 맞아야 한다
키워드 대체 검색에서 낱말 두 개 이하의 질문은 모든 낱말이 맞는 절만, 그보다 긴 질문은 묶음에 있는 낱말 무게의 절반 이상이 맞는 절만 결과로 내야 한다(SHALL).

#### Scenario: 두 낱말 질문
- **GIVEN** 한 낱말만 있는 문서
- **WHEN** 두 낱말로 찾는다
- **THEN** 그 문서는 결과에 없다

### Requirement: 여러 표현을 합친다
`orai search`는 `--also`로 받은 같은 질문의 다른 표현마다 문서 순위를 매긴 뒤 순위 역수로 합쳐야 한다(SHALL). shelf 서버가 답하면 표현마다 질의를 보낸다. 메일과 코드는 첫 표현만 쓴다. 빈 표현은 무시한다.

#### Scenario: 다른 표현이 문서를 찾는다
- **GIVEN** 첫 표현으로는 맞는 문서가 없다
- **WHEN** `--also`로 그 문서에 있는 표현을 더한다
- **THEN** 그 문서가 결과에 나오고 weak 알림이 없다

### Requirement: 약한 결과를 알린다
키워드 결과가 없거나 첫 결과가 질문 무게의 60%에 못 미치면 `orai search`는 `note: weak match`와 어느 문서에도 없는 낱말, 그리고 `--also`로 다시 찾는 법을 알려야 한다(SHALL).

#### Scenario: 없는 낱말
- **WHEN** 어느 문서에도 없는 낱말이 들어간 질문으로 찾는다
- **THEN** `weak match: nothing matched; no document contains <그 낱말>`과 `--also` 안내가 나온다
