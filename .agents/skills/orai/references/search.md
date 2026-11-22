<!-- orai:generated — `orai setup`이 관리한다. 프로젝트 규칙은 AGENTS.md나 역할 지침에 둔다. -->

# 검색 결과 자세히 읽기

평소에는 [SKILL.md](../SKILL.md)의 "찾기와 읽기"로 충분하다. 결과가 이상하거나 소스 하나를 깊이 볼 때 읽는다.

## 소스

| 소스 | 찾는 곳 | 찾는 방법 | 결과 한 건 |
|---|---|---|---|
| `docs` | `orai.toml`의 `[integrations.shelf]` `collections` 폴더 | 내장 키워드 검색. QMD를 켠 프로젝트는 shelf 서버(키워드+의미) | `[docs] <경로>:<줄>`, 제목, 점수, `context`, 맞는 줄 |
| `mail` | 내 메일함의 받은 메시지와 보낸 메시지 | 키워드 | `[mail] <ID> 보낸 이 → 받는 이 (received/sent, thread)`, 맞는 줄 |
| `code` | 이 checkout의 코드 그래프(`.codegraph/`) | CodeGraph 심볼 검색 | `[code] <종류> <이름> <경로>:<시작>-<끝>`, 시그니처, 설명 |

설정하지 않은 소스는 `not searched`로 나오고 문제로 세지 않는다. `--in`으로 직접 고른 소스가 준비되지 않았으면 `FAILED`다.

## 상태 줄과 조치

| 상태 줄 | 뜻 | 조치 |
|---|---|---|
| `docs: built-in keyword search` | 내장 검색이 답했다 | 없음 |
| `docs: keyword + meaning (shelf :<포트>)` | shelf 서버가 답했다 | 없음 |
| `docs: keyword scan of the files … Fix: …` | 서버가 없거나 꺼졌거나 다른 프로젝트 것이거나, 색인에 없는 묶음이 있다 | 줄에 적힌 명령. 샌드박스의 접근 거부는 서버가 꺼졌다는 근거가 아니다 |
| `note: collection folder(s) missing: …` | 선언한 묶음 폴더가 없어 그 묶음은 찾지 않았다 | 폴더를 만들거나 `collections`를 고친다 |
| `note: N document(s) changed since the last sync` | 색인이 문서보다 뒤처졌다 | `orai shelf sync` |
| `note: N message(s) not received yet also match` | 받지 않은 메시지 중에도 맞는 것이 있다 | `orai inbox` |
| `note: N file(s) changed since the graph was synced` | 코드 그래프가 파일보다 뒤처졌다 | `orai code sync` |
| `code: FAILED — no code graph is built …` | 이 checkout에 그래프가 없다 | `orai code init` (만들지는 사용자가 정한다) |

## MCP 도구

역할 세션에는 QMD를 켠 프로젝트면 MCP 서버 `shelf`가, 그 checkout에 코드 그래프가 있으면 `codegraph`가 연결된다. Claude Code에서는 이 도구들이 결과를 그대로 보여 주므로 써도 된다. Codex는 MCP 결과 중 일부(발췌, `context`, 본문)를 모델에 넘기지 않으므로 `orai search`와 `orai read`를 쓴다. 어느 쪽이든 원문은 파일로 확인한다.

## 기계가 읽을 출력

`orai search … --json`, `orai read … --json`. 필드 이름은 출력에 있다.
