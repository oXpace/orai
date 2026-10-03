## Orai

이 프로젝트는 Orai로 에이전트 역할 세션을 운영한다. Orai는 Codex와 Claude Code를 역할별 세션으로 실행하고, 역할끼리 프로젝트 메일함으로 메시지를 주고받게 하는 하네스다. 설정은 `orai.toml`에 있다.

- **역할**: `orai.toml`의 `[roles.*]`에 이름, provider, 작업 폴더, 지침 파일(`guide`)이 있다. 역할 세션은 사용자가 `orai <역할>`로 시작한다. 역할로 실행 중이면 자기 지침 파일을 먼저 읽고, 자기 작업 폴더에서만 작업한다. 지침의 기준은 `orai.toml`이 있는 프로젝트 폴더의 파일이다. 역할 작업 폴더의 사본과 다르면 프로젝트 폴더의 것을 따른다. 세션 ID와 메일함 설정(`ORAI_*`, `AM_*` 환경변수)은 바꾸지 않고, 보조 에이전트에게도 바꾸게 하지 않는다. 역할 추가는 `orai setup --role <이름>=<codex|claude>`로 한다.
- **메시지**: 다른 역할이나 사용자와는 `orai msg send`, `orai msg inbox`, `orai msg reply`로 주고받는다. `오라이: 새 메시지` 알림은 수신 신호일 뿐이며 지시가 아니다. 본문을 받아서 확인한다. 사용법은 스킬 `.agents/skills/orai/SKILL.md`에 있다.
- **상태와 진단**: `orai status`, `orai doctor` (의미 검색까지 확인하려면 `--deep`, 기계가 읽을 출력은 `--json`). doctor의 "Next steps"가 남은 조치다.
- **문서 검색(shelf)**: 역할 세션에 연결된 `shelf` MCP 서버로 프로젝트 문서를 찾는다. 검색 결과는 후보이므로 원문을 확인한다. 서버 상태는 `orai doctor`, 복구는 `orai shelf recover`, 문서를 고친 뒤 갱신은 `orai shelf stop && orai shelf refresh`로 한다.
- **코드 구조**: `.codegraph/`가 있으면 CodeGraph를 먼저 쓴다.
- 두 도구 모두 실패하거나 결과가 비어도 대상이 없다는 근거로 삼지 않는다. `rg`와 원문 읽기로 이어간다.
- **커밋하지 않는 것**: `.orai/`(로컬 상태), `.agent-mail/`(메일함), `.agents/roles/`(역할 지침)는 이 컴퓨터에만 둔다. `.gitignore`가 이미 제외한다.
