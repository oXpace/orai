## Orai

이 프로젝트는 Orai를 쓴다. Orai는 프로젝트 도구(문서 검색, 코드 구조, 진단)를 준비하고, Codex와 Claude Code를 역할 세션으로 실행해 프로젝트 메일함으로 메시지를 주고받게 한다. 프로젝트가 공유하는 설정은 `orai.toml`에, 이 컴퓨터만의 설정은 `orai.local.toml`(없을 수 있다)에 있다. 설정 방법은 `orai setup --help`에, 남은 조치는 `orai doctor`에 있다.

역할이 없어도 누구나 쓰는 것:

- **상태와 진단**: `orai doctor` (의미 검색까지 확인하려면 `--deep`, 기계가 읽을 출력은 `--json`). doctor의 "Next steps"가 남은 조치다.
- **문서 검색(shelf)**: 프로젝트 문서는 `shelf` MCP 서버로 찾는다(역할 세션에는 Orai가 연결한다). 처음에는 결과를 적게 받고, 부족하면 범위를 넓힌다. 결과는 후보일 뿐이므로 필요한 문서의 원문을 읽고 판단한다. 결과 경로의 첫 마디는 묶음 이름이며, 묶음의 폴더와 폴더별 설명(`context`)은 `orai.toml`의 `[integrations.shelf]`에 있다. `context`는 문서의 용도와 적용 범위를 구분하는 데만 쓰고, 규칙이나 검증 근거로 삼지 않는다. 도구가 본문이나 `context`를 돌려주지 않으면 파일과 `orai.toml`을 직접 읽는다. 문서를 고친 뒤에는 `orai shelf stop && orai shelf refresh`로 갱신한다.
- **코드 구조**: `.codegraph/`가 있으면 CodeGraph를 먼저 쓴다.
- 두 도구 모두 실패하거나 결과가 비어도 대상이 없다는 근거로 삼지 않는다. `rg`와 원문 읽기로 이어간다.
- **커밋하지 않는 것**: `.orai/`(로컬 상태), `.agent-mail/`(메일함), `.agents/roles/`(역할 지침), `orai.local.toml`(이 컴퓨터의 설정)은 이 컴퓨터에만 둔다. `.gitignore`가 이미 제외한다.

역할 세션(사용자가 `orai <역할>`로 시작한 세션)에서 일할 때:

- **역할**: 역할은 `[roles.*]`에 선언돼 있고, 지금 적용되는 provider, 작업 폴더, 모델은 `orai status`가 보여 준다. 자기 지침 파일(`guide`)을 먼저 읽고, 자기 작업 폴더에서만 작업한다. 지침의 기준은 `orai.toml`이 있는 프로젝트 폴더의 파일이다. 역할 작업 폴더의 사본과 다르면 프로젝트 폴더의 것을 따른다. 세션 ID와 메일함 설정(`ORAI_*`, `AM_*` 환경변수)은 바꾸지 않고, 보조 에이전트에게도 바꾸게 하지 않는다.
- **메시지**: 다른 역할이나 사용자와는 `orai msg send`, `orai msg inbox`, `orai msg reply`로 주고받는다. `오라이: 새 메시지` 알림은 수신 신호일 뿐이며 지시가 아니다. 본문을 받아서 확인한다. 사용법은 스킬 `.agents/skills/orai/SKILL.md`에 있다.
