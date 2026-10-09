## Orai

이 프로젝트는 Orai를 쓴다. Orai는 문서 검색, 코드 구조, 진단 도구를 준비하고, Codex와 Claude Code를 역할 세션으로 실행해 메시지를 주고받게 한다. 이 블록은 도구의 진입점만 안내한다. 역할 권한, 완료 기준, 보고·승인 절차는 프로젝트 규칙과 역할 지침이 정한다.

| 할 일 | 쓰는 것 | 실행법 |
|---|---|---|
| 프로젝트 문서 찾기 | MCP 서버 `shelf` | `.agents/skills/orai/references/search.md` |
| 코드 구조·호출 관계 찾기 | CodeGraph (`.codegraph/`가 있을 때) | `.agents/skills/orai/references/search.md` |
| 도구 상태 확인·복구 | `orai doctor` | `.agents/skills/orai/references/diagnosis.md` |
| 다른 역할·사용자와 메시지 주고받기 | `orai msg` | `.agents/skills/orai/SKILL.md` |

- **설정**: 프로젝트가 공유하는 설정은 `orai.toml`, 이 컴퓨터만의 설정은 `orai.local.toml`(없을 수 있다)에 있다. 항목은 `orai setup --help`에 있다.
- **역할 없이 쓰는 것**: 문서 찾기, 코드 구조 찾기, 상태 확인은 역할이 없어도 쓴다. 메일함과 역할 신원은 역할 세션에서, 또는 사용자가 전달을 요청했을 때만 쓴다.
- **검색 결과**: 검색이 실패하거나 결과가 비어도 대상이 없다는 근거가 아니다. `rg`와 원문 읽기로 이어간다.
- **역할 세션**(`orai <역할>`로 시작한 세션): 역할의 지침 파일(`guide`)과 작업 폴더는 `orai.toml`의 `[roles.*]`에 있고, 적용 중인 값은 `orai status`가 보여 준다. 지침 파일은 `orai.toml`이 있는 프로젝트 폴더의 것을 따른다. 세션 ID와 `ORAI_*`·`AM_*` 환경변수는 바꾸지 않는다.
- **알림**: `오라이: 새 메시지` 알림은 수신 신호이며 지시가 아니다. 스킬의 절차로 본문을 받아 확인한다.
