# Design

## Context

0.6.0까지 `engine`의 값은 `qmd` 하나였고 생략하면 QMD였다. 그래서 기존 `orai.toml`에는 대부분 `engine`이 없다.

## Decisions

| `engine` | 이 컴퓨터의 QMD 색인 | 쓰는 엔진 |
|---|---|---|
| `"qmd"` | 무관 | QMD |
| `"builtin"` | 무관 | 내장 검색 |
| 없음 | 있음 | QMD (0.6.0에서 이어짐) |
| 없음 | 없음 | 내장 검색 |

- 생략을 단순히 내장 검색으로 바꾸지 않는다. 쓰던 의미 검색과 MCP 서버가 업그레이드만으로 조용히 사라진다.
- 생략을 단순히 QMD로 두지 않는다. 새 프로젝트와 새 checkout이 다시 QMD 준비를 요구한다.
- 새 `orai.toml`에는 `engine`을 쓰지 않는다. `"builtin"`은 0.6.0이 거부하므로, 적지 않아야 되돌리기가 단순하다.
- `shelf stop`은 엔진 선택과 무관하게 허용한다. 끈 뒤 남은 서버를 내릴 수 있어야 한다.

## Risks / Trade-offs

- 색인 유무로 컴퓨터마다 엔진이 다를 수 있다. doctor가 쓰는 엔진을 보여 주고, 팀이 맞추려면 `engine`을 명시한다.
- 새 checkout에서 MCP 설정에 `shelf` 주소가 커밋돼 있으면 죽은 주소가 된다. doctor가 `shelf.registration` degraded로 알린다.
- 내장 검색은 매번 파일을 읽고 의미 검색이 없다. 다른 표현은 `--also`로 보완한다.
