# Proposal

## Why

shelf 서버가 없을 때 `orai search`의 키워드 대체 검색은 문서 전체를 낱말 수로만 매겨, 흔한 낱말이 많은 긴 문서(`docs/history.md`)가 정답 문서보다 앞에 왔다. 사용자가 QMD 대안으로 가져온 wikimap 1.5.1(MIT)을 검토한 결과, 엔진으로 쓰지는 않고 랭킹 방식만 Go로 옮기기로 했다(코드는 옮기지 않음).

## What Changes

- 키워드 대체 검색이 Markdown 문서를 제목으로 절을 나누고, 낱말을 IDF로 무게를 주며, 문서 제목·경로·절 제목과 한 줄에 모인 낱말을 더 무겁게 센다. 결과는 문서마다 가장 나은 절의 가장 나은 줄을 가리킨다(`문서 제목 › 절 제목`).
- 한글 낱말 꼬리(끝 음절 한두 개)와 섞인 낱말의 라틴 문자 부분(`hook이` → `hook`)도 맞춘다. 메일 검색도 같은 낱말 형태를 쓴다.
- `orai search --also <다른 표현>`: 문서는 표현마다 순위를 매겨 순위 역수(RRF)로 합치고, shelf 서버에는 표현마다 lex+vec 질의를 보낸다. 메일과 코드는 첫 표현만 쓴다.
- 결과가 약하면 `note: weak match …`와 어느 문서에도 없는 낱말, `--also` 재시도를 알린다.
- 하지 않은 것: wikimap의 답 메모·연결 층, 색인 파일, HTML·PDF.

## Capabilities

### New Capabilities

(없음)

### Modified Capabilities

- `agent-search`: 키워드 대체 검색의 순위, 여러 표현 합치기, 약한 결과 알림을 더한다.

## Impact

- 코드: `internal/lexical/docs.go`(새), `internal/lexical/lexical.go`, `internal/search`, `internal/cli/harness.go`·`args.go`.
- 문서: `docs/architecture.md`, `docs/operations.md`, `docs/history.md`, 스킬 템플릿.
- 구현: PR #7로 병합(5f36abb). 이 change는 병합 뒤 기록으로 작성했다.
