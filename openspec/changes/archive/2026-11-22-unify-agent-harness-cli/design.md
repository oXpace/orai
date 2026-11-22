# Design

## Context

검색은 MCP `shelf` 하나였고, CodeGraph는 진단만 했다. 설계 합의 문서: https://claude.ai/artifact/QNzBZiV9GJWtiV487xyUyZ (C안).

## Goals / Non-Goals

**Goals:**
- provider와 무관하게 같은 검색 결과가 모델에 닿는다.
- 에이전트가 색인 갱신을 고르지 않아도 된다.
- "알림은 ID만, 한 번만 수신" 계약을 어느 명령이 먼저 닿아도 지킨다.

**Non-Goals:**
- CodeGraph를 Go로 다시 구현하지 않는다(다중 언어 파서 의존성).
- 메일을 색인하지 않는다.
- 소스를 한 순위로 섞지 않는다.

## Decisions

- **입구는 CLI, MCP는 보조.** 셸 출력은 어느 provider든 그대로 전달되지만 MCP 결과는 provider가 일부만 넘길 수 있다. 대안(MCP 응답 보강)은 provider마다 결과가 달라 버렸다.
- **소스별 출력.** 점수 체계가 달라 섞으면 순위가 의미를 잃는다. 소스마다 상태 줄을 먼저 찍어 실패와 빈 결과를 구분하게 한다.
- **메일은 키워드로 직접 찾는다.** 역할별 메일함을 공유 색인에 넣으면 신원 경계가 흐려진다. 받지 않은 메시지는 개수만 알린다.
- **대체 검색은 실패로 보고한다.** 키워드 훑기 결과는 그대로 주되 종료 코드 1과 이유를 붙여, 빈 결과를 "없다"로 읽지 않게 한다.
- **자동 sync는 런처가 한다.** fsnotify로 묶음 폴더를 보고 3초 조용하면 sync한다. shelf 잠금이 여러 역할의 순서를 정하고, 표식으로 이미 반영된 변경을 건너뛴다. 만들지 않은 shelf는 만들지 않는다.
- **그래프는 checkout마다.** 역할 worktree의 코드는 그 checkout에만 있다. 세션 MCP는 `codegraph serve --mcp --path <작업 폴더>`로 주입하고, 서버의 파일 감시가 세션 중 갱신을 맡는다. 전역 에이전트 설정은 바꾸지 않는다.

## Risks / Trade-offs

- [전역에 사용자가 등록한 `codegraph`와 이름이 겹침] → 우선순위는 provider가 정하며 미검증. 문서에 적고 doctor로 확인할 항목으로 남긴다.
- [자동 sync가 QMD 임베딩 비용을 자주 씀] → debounce와 표식 비교로 줄이고, 출력은 `.orai/shelf/autosync.log`로 보낸다.
- [키워드 대체 검색의 한국어 품질] → 간단한 어간 처리만 한다. 의미 검색 품질은 별도 검증 대상이다.

## Migration Plan

`orai setup`이 AGENTS 블록과 스킬을 갱신한다. 역할 이름이 `search|read|code`인 프로젝트는 이름을 바꾼다. 코드 그래프를 쓰는 프로젝트는 `orai code init`을 실행하고, 실행 중인 역할 세션은 다시 연다.
