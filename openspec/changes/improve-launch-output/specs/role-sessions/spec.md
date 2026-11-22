## ADDED Requirements

### Requirement: 실행 안내는 세 줄로 요약한다
역할을 시작할 때 Orai는 stderr에 프로젝트·역할·대화(새 대화 또는 이어서와 ID 앞 8자), provider·모델·effort, 작업 폴더·새 메시지 수를 세 줄로 내야 한다(SHALL). 검색 엔진과 hook 설명은 내지 않는다.

#### Scenario: 저장된 대화로 시작한다
- **GIVEN** 모델, effort, 별도 작업 폴더가 있고 새 메시지가 2개인 Claude 역할 staff와 저장된 세션
- **WHEN** `orai staff`를 실행한다
- **THEN** `● <프로젝트> › staff  이어서 <ID 8자>`, `Claude · <모델> · <effort>`, `<폴더> · 새 메시지 2` 세 줄이 나온다

#### Scenario: 선택 값이 없다
- **GIVEN** 모델과 effort가 없고 작업 폴더가 `.`인, 저장된 세션 없는 역할
- **WHEN** 역할을 시작한다
- **THEN** `새 대화`, `기본 모델`, `프로젝트 폴더`로 표시되고 effort 칸은 없다

#### Scenario: 메일함을 읽지 못한다
- **GIVEN** 새 메시지 수를 읽는 데 실패한다
- **WHEN** 역할을 시작한다
- **THEN** 0으로 표시하지 않고 `새 메시지 확인 실패`로 표시한다

#### Scenario: 메일함을 더한다
- **GIVEN** 설정에 있으나 메일함이 없는 역할 qa
- **WHEN** 다른 역할을 시작한다
- **THEN** 세 줄 뒤에 `메일함 추가: qa`가 나오고 절대 경로는 나오지 않는다

#### Scenario: dry-run
- **WHEN** `orai run <역할> --dry-run`을 실행한다
- **THEN** stdout의 JSON만 나오고 실행 안내는 나오지 않는다

### Requirement: 종료 안내는 다음 행동을 알린다
provider가 끝나면 Orai는 저장된 상태와 작업 폴더를 다음 실행과 같은 기준으로 판정해, 이어갈 수 있으면 실행 명령을, 대화를 이어갈 수 없으면 `--fresh` 명령을, 실행 자체가 막히면 `orai doctor`를 알려야 한다(SHALL). hook 승인 안내는 이번 실행의 캡처가 없고 provider가 정상 종료한 Codex 역할에만 붙인다. 경고는 실제로 생긴 문제만 한 줄로 낸다.

#### Scenario: 이어갈 수 있다
- **GIVEN** 다음 실행의 재개 판정을 통과하는 상태
- **WHEN** provider가 끝난다
- **THEN** `○ <프로젝트> › <역할> 종료 · 이어가기: orai <역할>`이 나온다

#### Scenario: Codex 대화를 이어갈 수 없다
- **GIVEN** 세션 신원이 캡처되지 않은 새 Codex 대화
- **WHEN** provider가 끝난다
- **THEN** `! 이어갈 수 없음 → orai <역할> --fresh 후 hook 승인`이 나온다

#### Scenario: 캡처는 됐지만 transcript가 없다
- **GIVEN** 이번 실행에서 캡처된 Codex 신원의 transcript 파일이 없다 (예: clear 직후 종료)
- **WHEN** provider가 끝난다
- **THEN** `! 이어갈 수 없음 → orai <역할> --fresh`가 나오고 hook 안내는 나오지 않는다

#### Scenario: provider가 오류로 끝난다
- **GIVEN** 캡처 전에 0이 아닌 코드로 끝난 Codex 역할
- **WHEN** 종료 안내를 낸다
- **THEN** 원인을 hook으로 단정하지 않고 hook 안내를 내지 않는다

#### Scenario: 다음 실행이 막힌다
- **GIVEN** 세션 중에 작업 폴더가 없어졌거나, 선언한 브랜치와 달라졌거나, 상태 파일을 읽지 못한다
- **WHEN** provider가 끝난다
- **THEN** `! 다시 실행할 수 없음 → orai doctor`가 나오고 `--fresh`나 이어가기는 나오지 않는다

#### Scenario: Claude 대화를 이어갈 수 없다
- **GIVEN** 재개 판정을 통과하지 못하는 Claude 상태
- **WHEN** provider가 끝난다
- **THEN** `! 이어갈 수 없음 → orai <역할> --fresh`가 나오고 Codex 안내는 나오지 않는다

#### Scenario: 재개한 Codex에서 hook이 실행되지 않았다
- **GIVEN** 재개할 수 있지만 이번 실행의 캡처가 없는 Codex 역할
- **WHEN** provider가 끝난다
- **THEN** `! 이번엔 알림 꺼짐 → 다음 실행에서 hook 승인`이 나온다

#### Scenario: 재개한 Claude에서 캡처가 없다
- **GIVEN** 재개할 수 있지만 이번 실행의 캡처가 없는 Claude 역할
- **WHEN** provider가 끝난다
- **THEN** 알림이 꺼졌다는 경고는 나오지 않는다

### Requirement: 실행 안내는 터미널과 quiet를 따른다
Orai는 안내를 내는 스트림(실행 안내는 stderr, 항목표는 stdout)이 터미널이고 `NO_COLOR`가 비어 있으며 `TERM`이 `dumb`이 아닐 때만 색을 써야 한다(SHALL). `orai run`과 단축형의 `--quiet`, `-q`는 경고와 오류만 남긴다.

#### Scenario: 색을 쓰지 않는다
- **GIVEN** 출력 스트림이 터미널이 아니거나, `NO_COLOR`가 있거나, `TERM=dumb`이다
- **WHEN** 실행 안내를 낸다
- **THEN** ANSI 이스케이프가 없다

#### Scenario: quiet와 도움말을 같이 준다
- **WHEN** `orai <역할> --fresh -q --help`를 실행한다
- **THEN** 도움말만 나오고 provider를 실행하거나 상태를 바꾸지 않는다

#### Scenario: quiet로 실행한다
- **WHEN** `orai <역할> --quiet`로 실행한다
- **THEN** 시작·종료 안내는 없고, 문제가 있으면 경고 한 줄은 나온다

### Requirement: 역할 상태는 항목표로 보여 준다
`orai status <역할>`은 상태·실행·대화·폴더·알림·메일을 항목표로 내야 한다(SHALL). 값과 판정은 `orai status`의 JSON과 같되 전달 오류는 이번 실행의 것만 보여야 하고, 인자 없는 `orai status`의 JSON 출력은 바꾸지 않아야 한다(MUST). 검색 엔진은 표시하지 않는다.

#### Scenario: 실행 중인 역할
- **GIVEN** 알림이 준비된 실행 중인 역할과 새 메시지 2개
- **WHEN** `orai status <역할>`을 실행한다
- **THEN** `실행 중`, provider·모델·effort, 세션 ID 앞 8자, 작업 폴더, `준비됨`, `새 메시지 2`가 항목마다 한 줄로 나온다

#### Scenario: 멈춘 역할
- **GIVEN** 이어갈 수 있는 멈춘 역할
- **WHEN** `orai status <역할>`을 실행한다
- **THEN** 상태에 `멈춤 · 이어가기: orai <역할>`이 나오고 알림은 `-`로 나온다

#### Scenario: 이어갈 수 없는 멈춘 역할
- **GIVEN** 재개 판정이나 작업 폴더 검사를 통과하지 못하는 멈춘 역할
- **WHEN** `orai status <역할>`을 실행한다
- **THEN** 이어가기 대신 종료 안내와 같은 다음 행동이 나온다

#### Scenario: 실행한 적 없는 역할
- **GIVEN** 저장된 상태가 없고 실행할 수 있는 역할
- **WHEN** `orai status <역할>`을 실행한다
- **THEN** `멈춤 · 시작: orai <역할>`과 빈 대화(`-`)가 나온다

#### Scenario: 알림 문제
- **GIVEN** 실행 중이지만 알림이 준비되지 않았거나 마지막 전달 오류가 있다
- **WHEN** `orai status <역할>`을 실행한다
- **THEN** 알림 항목에 확인된 상태나 오류가 나오고 `준비됨`으로 표시하지 않는다

#### Scenario: 이전 실행이 남긴 전달 오류
- **GIVEN** 실행 중인 역할과 이전 실행의 nonce로 기록된 전달 오류
- **WHEN** `orai status <역할>`을 실행한다
- **THEN** 그 오류를 보이지 않고 알림은 `준비 안 됨`으로 나온다

#### Scenario: 없는 역할
- **WHEN** 설정에 없는 이름으로 `orai status <이름>`을 실행한다
- **THEN** 설정된 역할 목록과 함께 종료 코드 2로 끝난다

#### Scenario: 인자 없는 status
- **WHEN** `orai status`를 실행한다
- **THEN** 이전과 같은 JSON이 나온다
