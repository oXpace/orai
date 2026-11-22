# role-sessions Specification

## Purpose

`orai <역할>`(= `orai run <역할>`)로 Codex·Claude Code를 역할 세션으로 실행할 때, 그 역할의 대화를 정확히 이어 가고 신원이 다른 실행에 덮어쓰이지 않게 한다.

## Requirements

### Requirement: 저장된 세션은 정확한 ID로만 재개한다
역할에 저장된 세션이 있으면 Orai는 캡처한 세션 ID로 재개해야 한다(SHALL). provider, 작업 폴더, 기록된 transcript 파일이 모두 일치할 때만 재개하고, 하나라도 다르면 시작을 거부해야 한다(MUST). "가장 최근 대화"를 추측하거나 provider의 writer lock을 지워 복구하지 않는다.

#### Scenario: 저장된 ID로 재개한다
- **GIVEN** 세션 ID와 transcript가 기록된 역할
- **WHEN** `orai run <역할>`을 실행한다
- **THEN** provider 인자에 그 ID의 재개(Codex `resume <ID>`, Claude `--resume <ID>`)가 들어간다

#### Scenario: transcript가 기록과 다르다
- **GIVEN** 기록된 transcript 파일이 없거나 다른 파일이다
- **WHEN** `orai run <역할>`을 실행한다
- **THEN** provider를 실행하지 않고 오류로 끝나며 `--fresh`를 안내한다

### Requirement: 새 대화는 --fresh로만 시작한다
저장된 세션이 있는 역할을 `orai run`으로 실행할 때 새 대화는 `--fresh`를 줄 때만 시작해야 한다(SHALL). 실행 중인 역할 세션 안의 clear로 바뀐 대화는 신원 캡처 요구에 따라 이어받는다. 두 경우 모두 이전 상태는 지우지 않고 `history/`로 옮긴다.

#### Scenario: --fresh로 다시 시작한다
- **GIVEN** 저장된 세션이 있는 역할
- **WHEN** `orai run <역할> --fresh`를 실행한다
- **THEN** 새 세션이 시작되고 이전 상태 파일이 `history/`에 남는다

#### Scenario: clear한 뒤 다시 실행한다
- **GIVEN** clear로 바뀐 세션 ID와 transcript가 캡처된 뒤 종료한 역할
- **WHEN** `--fresh` 없이 `orai run <역할>`을 실행한다
- **THEN** clear 뒤의 ID로 재개하고 clear 전 상태는 `history/`에 남아 있다

### Requirement: 신원은 같은 실행의 nonce로만 캡처한다
SessionStart hook이 보낸 세션 신원은 환경의 run nonce가 상태 파일의 nonce와 같고, 작업 폴더가 역할 작업 폴더와 같을 때만 기록해야 한다(SHALL). 이미 기록된 ID와 다른 ID는 hook의 source가 `clear`이고, transcript 경로가 있을 때만 받아 ID와 transcript를 함께 교체해야 한다(SHALL). 그 밖의 경우는 기록하지 않고 거부해야 한다(MUST). 멈춘 실행으로의 캡처도 거부한다.

#### Scenario: 이전 실행이 남긴 hook
- **GIVEN** 상태 파일의 nonce와 다른 nonce를 가진 hook 입력
- **WHEN** `_capture`가 실행된다
- **THEN** 신원이 기록되지 않고 기존 신원이 그대로다

#### Scenario: 다른 폴더에서 온 hook
- **GIVEN** nonce는 같지만 cwd가 역할 작업 폴더와 다르다
- **WHEN** `_capture`가 실행된다
- **THEN** 캡처가 거부된다

#### Scenario: 기록된 ID와 다른 ID
- **GIVEN** 이미 세션 ID가 기록된 실행
- **WHEN** `clear`가 아닌 source와 다른 세션 ID로 `_capture`가 실행된다
- **THEN** 캡처가 거부되고 기록된 ID가 유지된다

#### Scenario: 같은 실행에서 clear한다
- **GIVEN** 신원 캡처가 끝난 실행
- **WHEN** 같은 nonce와 cwd, source `clear`, 새 ID와 transcript 경로로 `_capture`가 실행된다
- **THEN** ID와 transcript가 함께 바뀌고 이전 상태가 `history/`에 남으며 이후 Codex 알림은 새 ID로 간다

#### Scenario: 시작 hook보다 빠른 clear
- **GIVEN** ID가 정해진 채 시작했지만 이번 실행의 첫 캡처가 끝나지 않았다
- **WHEN** 같은 nonce와 cwd, source `clear`, 다른 세션 ID와 transcript 경로로 `_capture`가 실행된다
- **THEN** 새 신원으로 교체되고 캡처가 끝난다

#### Scenario: transcript 경로가 없는 clear
- **GIVEN** 신원 캡처가 끝난 실행
- **WHEN** transcript 경로가 빈 clear hook이 다른 ID로 온다
- **THEN** 캡처가 거부되고 기존 ID와 transcript가 유지된다

### Requirement: 한 역할은 한 번에 하나만 실행된다
같은 프로젝트의 같은 역할이 이미 실행 중이면 두 번째 실행을 거부해야 한다(MUST). 다른 프로젝트의 같은 이름 역할은 막지 않는다.

#### Scenario: 같은 역할을 두 번 실행한다
- **GIVEN** `orai run lead`가 실행 중이다
- **WHEN** 같은 프로젝트에서 다시 `orai run lead`를 실행한다
- **THEN** "already running" 오류로 거부된다

### Requirement: 역할 세션의 환경은 그 역할에 고정된다
역할 세션의 자식 환경에서 Orai는 호출 셸의 `AM_*`, `AMQ_GLOBAL_ROOT`, `ORAI_*`를 지우고 그 역할의 `ORAI_ROLE`, `ORAI_PROJECT`, `ORAI_SESSION`, `ORAI_RUN_NONCE`, `ORAI_MAIL_ROOT`와 AMQ 호환 `AM_ROOT`, `AM_ME`, `AM_SESSION`을 설정해야 한다(SHALL). provider는 프로세스 인자로만 설정하고 전역 설정 파일을 바꾸지 않는다.

#### Scenario: 다른 역할의 환경에서 실행한다
- **GIVEN** 호출 셸에 다른 역할의 `AM_ME`와 `ORAI_ROLE`이 있다
- **WHEN** `orai run lead`를 실행한다
- **THEN** 자식 환경의 `AM_ME`와 `ORAI_ROLE`은 `lead`다

### Requirement: clear 뒤에는 역할 문맥을 다시 안내한다
clear로 신원을 교체한 hook은 역할, 프로젝트 폴더, 지침 경로와 함께 지침을 읽고 `orai inbox`를 확인하라고 안내해야 한다(SHALL). 지침 본문과 메시지 본문은 담지 않고(MUST NOT), 하던 작업을 이어가라고 안내하지 않는다(MUST NOT).

#### Scenario: clear 뒤 첫 응답 전
- **WHEN** clear로 신원이 교체된다
- **THEN** hook의 추가 문맥에 지침 경로와 `orai inbox`가 있고 "하던 작업을 이어간다"가 없다

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
