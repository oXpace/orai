## Orai

이 프로젝트의 역할 세션은 Orai로 실행한다. 역할과 연동 설정은 `orai.toml`에 있다.

- 메시지 수신·회신·전송(`orai msg`): `.agents/skills/orai/SKILL.md`
- 세션 상태와 진단: `orai status`, `orai doctor` (의미 검색까지 확인하려면 `--deep`)
- 문서 검색: 역할 세션에 연결된 `wiki-<프로젝트>` MCP 서버를 쓴다. 서버 상태는 `orai doctor`, 복구는 `orai wiki recover`로 한다.
- 코드 구조: `.codegraph/`가 있으면 CodeGraph를 먼저 쓴다.
- 두 도구 모두 실패하거나 결과가 비어도 대상이 없다는 근거로 삼지 않는다. `rg`와 원문 읽기로 이어간다.
