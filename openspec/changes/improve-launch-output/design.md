# Design

## Context

- Codex 0.171.0과 Claude Code 2.1.330 `tui: fullscreen`은 대체 화면을 쓴다. 시작 출력은 세션이 끝난 뒤에야 다시 보인다.
- Codex 알림은 이번 실행의 `captured_at`이 있어야 동작한다. Claude channel은 `channel_ready`만 있으면 동작한다.

## Decisions

### D1. 상황과 상태를 나눈다

- 결정: 실행·종료는 짧은 요약, `orai status <역할>`은 항목표로 낸다.
- 이유: 실행할 때는 무엇이 시작됐는지만, 상태를 볼 때는 항목별 값이 필요하다.
- 버린 대안: 실행할 때마다 항목표(길다), 인자 없는 `orai status`를 표로 변경(JSON을 읽는 쪽이 깨진다).

### D2. 경고는 종료 뒤에 낸다

- 결정: 이번 실행에서 실제로 생긴 문제만 종료 뒤 한 줄로 낸다.
- 이유: 실행 전 출력은 provider 화면에 가려진다.
- 버린 대안: `codex app-server`의 `hooks/list`로 사전 검사(experimental이고, Codex가 첫 화면에서 승인을 직접 묻는다).

### D3. 재개 판정은 하나다

- 결정: 종료 안내와 항목표의 다음 행동은 Launch가 쓰는 작업 폴더 검사와 `ValidateSaved`로 판정한다. 폴더·브랜치 문제는 `--fresh`로 풀리지 않으므로 `orai doctor`로 보낸다.
- 이유: 안내와 다음 실행의 판정이 어긋나지 않는다.

### D4. 알림 경고는 provider의 조건을 따른다

- 결정: 캡처가 없어 알림이 꺼졌다는 경고와 hook 승인 안내는 Codex에만, 정상 종료일 때만 낸다. 재개 가능 여부와 이번 실행의 캡처 여부는 따로 판정한다. clear 직후 종료하면 캡처됐어도 transcript가 없을 수 있다.
- 이유: Claude channel은 캡처 없이도 알림을 전달한다. provider가 오류로 끝났으면 캡처가 없는 원인을 알 수 없다.

### D5. 안내는 stderr, 색은 stderr 기준이다

- 결정: 실행 안내(메일함 추가 안내 포함)는 stderr로, 항목표는 stdout으로 낸다. 색은 그 파일이 문자 장치일 때만 쓴다.
- 이유: `--dry-run`의 stdout 계약을 지키고 의존성을 늘리지 않는다.
- 버린 대안: 터미널 UI 라이브러리.

### D6. 항목표와 JSON은 계산이 하나다

- 결정: 역할 하나의 `RoleStatus` 계산을 JSON과 항목표가 같이 쓴다. `delivery_ready`의 판정은 바꾸지 않는다.
- 이유: 두 출력이 다른 상태를 말하지 않는다.

## Risks

- `delivery_ready`는 Claude도 캡처를 요구한다. 캡처 없이 channel만 준비된 Claude 역할은 JSON과 항목표 모두에서 준비 안 됨으로 보인다.
- `orai status`는 지금까지 남는 인자를 무시했다. 이제 역할 이름으로 읽으므로 없는 이름은 종료 코드 2다.
