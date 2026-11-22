# Design

## Context

- Claude Code(2.1.330) `/clear`, `/new`: 즉시 `SessionStart source=clear`로 새 ID와 transcript 경로를 준다. 파일은 hook 뒤에 생길 수 있다.
- Codex(0.171.0) `/clear`: 다음 입력 때 `source=clear`로 새 ID를 준다. 그 전에는 새 ID를 알 수 없다.
- Codex `/new`: 다음 입력 때 `source=startup`으로 새 ID를 준다.
- 두 provider 모두 hook의 additionalContext를 새 대화에 넣는다.
- Codex는 hook의 matcher나 명령 경로가 바뀌면 다시 신뢰를 묻는다.

## Decisions

### 1. clear만 신원 교체로 인정한다

nonce, cwd, 실행 중(starting 또는 running), `source=clear`, transcript 경로가 모두 맞을 때 교체한다. Codex `/new`는 제외한다. `startup`의 다른 ID를 받으려면 신원 보호 규칙을 더 풀어야 하기 때문이다.

### 2. 첫 캡처 완료를 요구하지 않는다

빠른 clear는 진행 중인 시작 hook을 취소한다. `captured_at`을 요구하면 그 실행은 새 ID를 따라가지 못한다.

### 3. transcript 존재는 재개 때 본다

교체 시점에는 경로만 요구한다. 파일이 hook 뒤에 생길 수 있고, 재개 때 기존 검사가 파일을 확인한다.

### 4. notifier는 바꾸지 않는다

notifier는 전달마다 상태의 `session_id`를 읽는다. clear 전에 알린 미수신 메시지는 복원 문맥의 inbox 안내로 받는다.

### 5. clear에 한해 지침을 다시 안내한다

clear는 지침을 문맥에서 없앤다. "hook은 문서를 다시 읽게 하지 않는다"에 clear 예외를 두고, 경로만 안내하며 본문은 넣지 않는다.

## Consequences

- Codex는 clear 뒤 첫 입력 전까지 알림이 이전 thread로 가고, 그대로 종료하면 이전 대화로 재개된다.
- hook이 끝나기 전에 종료하면 교체되지 않을 수 있다.
- `Capture`는 hook의 순서를 판정하지 않는다. provider가 hook을 순서대로 실행하거나 앞 hook을 취소한다고 전제한다(Claude는 연속 clear와 종료 때 진행 중인 hook을 취소한다). 이 전제가 깨져 앞선 clear나 시작 hook이 늦게 쓰면 이전 대화로 되돌아간다.
- 교체한 transcript 파일이 끝내 생기지 않으면 다음 재개는 기존 규칙대로 거부된다. 이전 신원은 `history/`에 남는다.
- Codex 사용자는 hook을 한 번 다시 신뢰한다.
