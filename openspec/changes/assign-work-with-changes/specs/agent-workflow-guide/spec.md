## MODIFIED Requirements

### Requirement: 역할 사이 인계는 변경 이름으로 한다
안내는 작업을 맡길 때 `--kind todo --thread change/<이름>`으로 보내고, 본문에는 첫 줄 `change: <이름> task: <tasks.md의 번호>`와 만족시킬 시나리오만 적고 스펙 내용을 옮겨 쓰지 않게 해야 한다(SHALL). 답장은 같은 첫 줄로 시작하게 하고, 메일 검색 결과는 thread가 `change/<이름>`인 메시지만 그 작업으로 보게 해야 한다(MUST).

#### Scenario: todo 인계 지침
- **WHEN** 생성된 `workflow.md`의 "역할 사이에서" 절을 읽는다
- **THEN** `todo` 본문에 변경 이름과 시나리오만 적으라는 지침이 있다
- **AND** `--thread change/<이름>`, 첫 줄 형식, `openspec instructions apply`의 순서 id를 쓰지 말라는 지침이 있다

#### Scenario: 지난 맥락 찾기
- **WHEN** 생성된 `workflow.md`에서 지난 맥락을 찾는 방법을 읽는다
- **THEN** thread가 `change/<이름>`인 메시지만 그 작업으로 보라는 지침이 있다

## ADDED Requirements

### Requirement: change 사이의 순서를 적는다
안내는 change 사이의 선후를 `.openspec.yaml`의 `x-after`에 적고, 선행 change가 보관된 뒤에 맡기게 해야 한다(SHALL). Orai와 OpenSpec은 이 순서를 강제하지 않는다.

#### Scenario: 선행 change
- **WHEN** 생성된 `workflow.md`의 "역할 사이에서" 절을 읽는다
- **THEN** `x-after`와 보관된 뒤에 맡긴다는 규칙이 있다

### Requirement: 맡길 권한은 역할 지침에 둔다
change를 누가 맡기는지는 Orai 스킬이 아니라 생성되는 역할 지침 템플릿에 적어야 한다(SHALL).

#### Scenario: 역할 지침
- **WHEN** manager, engineer, 일반 역할의 지침 템플릿을 읽는다
- **THEN** 각각 change를 맡기는지, 누구에게서 받는지, 또는 프로젝트가 정할 자리가 있다
