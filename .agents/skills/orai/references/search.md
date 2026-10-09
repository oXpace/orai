<!-- orai:generated — `orai setup`이 관리한다. 프로젝트 규칙은 AGENTS.md나 역할 지침에 둔다. -->

# 프로젝트 문서와 코드 구조 찾기

역할이 없어도 쓴다.

## 문서 (shelf)

도구는 MCP 서버 `shelf`다. 역할 세션에는 Orai가 연결한다. 그 밖의 클라이언트에 등록하는 방법은 `orai shelf --help`에 있다.

1. 결과를 적게 받아 검색한다.
2. 부족하면 범위를 넓힌다.
3. 필요한 문서의 원문을 읽고 판단한다. 검색 결과는 후보다.

| 항목 | 내용 |
|---|---|
| 결과 경로 | `<묶음>/<묶음 안의 경로>`. 묶음의 폴더는 `orai.toml`의 `[integrations.shelf]` `collections`에 있다 |
| `context` | 그 폴더 문서의 용도와 적용 범위. `orai.toml`의 `[integrations.shelf.context]`에 선언한다. 규칙이나 검증 근거로 쓰지 않는다 |
| 도구가 본문이나 `context`를 돌려주지 않을 때 | 파일과 `orai.toml`을 직접 읽는다 |
| 문서를 고친 뒤 | `orai shelf sync` |
| 검색이 실패하거나 결과가 빌 때 | 대상이 없다는 근거가 아니다. `rg`와 원문 읽기로 이어가고, 서버 상태는 [diagnosis.md](diagnosis.md)로 확인한다 |

## 코드 구조 (CodeGraph)

`.codegraph/`가 프로젝트 폴더에 있을 때만 쓴다.

| 할 일 | 명령 |
|---|---|
| 심볼의 원문과 호출 관계 찾기 | `codegraph explore "<심볼 이름이나 질문>"` (MCP 도구가 연결돼 있으면 `codegraph_explore`) |
| 색인이 없거나 실패할 때 | `rg`와 원문 읽기 |

색인을 새로 만들지는 사용자가 정한다.
