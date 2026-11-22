# Tasks

## 1. macOS (보관한 change에서 넘어옴)

- [ ] 1.1 실제 QMD 서버 경유 `orai search` 결과와 상태 줄, `--also` 융합을 확인한다 (`unify-agent-harness-cli` 5.1, `rank-keyword-doc-search`)
- [ ] 1.2 역할 세션 중 문서를 고쳐 `.orai/shelf/autosync.log`에 sync가 남는지 확인한다 (`unify-agent-harness-cli` 5.2)
- [ ] 1.3 역할 세션에서 `codegraph` MCP 도구가 보이고 자기 worktree를 읽는지, 전역 `codegraph` 등록과의 우선순위를 확인한다 (`unify-agent-harness-cli` 5.3)
- [ ] 1.4 `orai setup`으로 실제 OpenSpec init과 doctor `openspec` 항목을 확인한다 (`default-spec-test-first` 4.3)

## 2. Linux 준비

- [ ] 2.1 Linux 호스트(배포판, 아키텍처)와 provider·도구 버전을 정하고 `docs/compatibility.md`에 적는다

## 3. Linux 실제 파일럿

- [ ] 3.1 `docs/operations.md` "실제 파일럿" 1~4단계를 수행하고 단계별 통과·실패를 기록한다
- [ ] 3.2 `orai shelf init`·`orai doctor --deep`으로 QMD CPU fallback 검색을 확인한다
- [ ] 3.3 `orai code init`·`orai search --in code`와 역할 세션 `codegraph` MCP를 확인한다
- [ ] 3.4 재부팅 뒤 `orai shelf recover`와 역할 재개를 확인한다

## 4. 기록

- [ ] 4.1 `docs/history.md`에 날짜·버전·통과·미검증 범위를 남기고 README 상태 표를 고친다. fixture 통과와 실제 동작을 나눠 적는다
