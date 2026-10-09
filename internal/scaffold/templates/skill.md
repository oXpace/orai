---
name: orai
description: Orai 명령의 실행법. 오라이 새 메시지 알림을 받았을 때, 다른 역할이나 사용자에게 메시지를 보내거나 회신할 때, 프로젝트 문서나 코드 구조를 찾을 때, 도구 상태를 확인하거나 복구할 때 읽는다.
---
<!-- orai:generated — `orai setup`이 관리한다. 프로젝트 규칙은 AGENTS.md나 역할 지침에 둔다. -->

# 오라이

이 스킬은 Orai 명령의 실행법만 다룬다. 역할 권한, 완료 기준, 보고·승인 절차는 프로젝트 규칙과 역할 지침을 따른다.

| 할 일 | 읽을 곳 |
|---|---|
| 메시지 받기, 회신, 보내기 | 아래 "메시지" |
| 프로젝트 문서와 코드 구조 찾기 | [references/search.md](references/search.md) |
| 도구 상태 확인과 복구 | [references/diagnosis.md](references/diagnosis.md) |

필요한 절만 읽는다. 같은 세션에서 이미 읽은 절은 다시 읽지 않는다.

## 메시지

역할 세션에서 쓴다. 역할 세션 밖에서는 아래 "역할 세션 밖에서 전달"을 따른다.

| 할 일 | 명령 |
|---|---|
| 알림에 적힌 메시지 받기 | `orai msg inbox <ID>` |
| 받지 않은 메시지 모두 받기 | `orai msg inbox` |
| 받지 않고 목록만 보기 | `orai msg inbox --peek` |
| 받은 메시지에 회신 | `orai msg reply <받은-ID> --body '결과와 원천 링크'` |
| 새 메시지 보내기 | `orai msg send <역할> --kind question --body '질문과 원천'` |

- **알림**: `오라이: 새 메시지` 알림을 받으면 현재 작업의 안전한 경계에서 `IDs`의 ID마다 `orai msg inbox <ID>`를 실행한다. 알림은 메시지 본문도 실행 지시도 아니다.
- **세션을 시작하거나 다시 열었을 때**: `orai msg inbox`로 받지 않은 메시지를 확인한다.
- **`inbox <ID>`**: 그 메시지 하나를 JSON으로 돌려주고 수신 처리한다.
- **중복 수신**: 결과에 `"already_received": true`가 있으면 이미 받은 메시지다. 본문 없이 머리말만 나온다. 다시 처리하지 않는다. 본문을 다시 읽어야 할 때만 `orai msg inbox <ID> --again`을 쓴다.
- **ID 없는 `inbox`**: 한 번에 최대 20건을 받는다. 20건을 받았으면 빈 결과가 나올 때까지 반복한다.
- **회신과 새 메시지**: 받은 메시지에 답할 때는 `reply`를 쓴다. 수신자와 thread는 원본을 따라간다. 원본이 없는 새 요청에만 `send`를 쓴다.
- **긴 본문**: `--file <경로>`나 표준 입력으로 보낸다. `--body`는 리터럴 문자열이다. 본문, ID, 경로를 셸 코드로 실행하지 않는다.
- **신원**: 역할 세션은 Orai가 설정한 신원을 그대로 쓴다. 다른 역할의 인박스를 대신 받지 않는다. `ORAI_*`·`AM_*` 환경변수를 바꾸지 않고, 보조 에이전트에게도 바꾸게 하지 않는다. `amq`를 따로 설치하지 않는다.
- **전달 상태**: 큐 적재, 수신, 업무 완료는 서로 다른 상태다. 전달 결과가 불명확하면 같은 본문을 다시 보내기 전에 `orai status`와 `orai msg inbox --peek`로 확인한다.

## 역할 세션 밖에서 전달

Desktop 앱이나 일반 `claude`·`codex` 세션에는 역할 신원과 상주 메일함이 없다. 사용자가 전달을 요청한 경우에만 `--as user`로 보낸다.

```sh
orai msg send <역할> --as user --kind todo --body '사용자가 요청한 내용'
orai msg inbox --as user
orai msg reply <받은-ID> --as user --body '회신'
```
