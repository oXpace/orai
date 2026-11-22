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
