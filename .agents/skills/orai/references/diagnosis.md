<!-- orai:generated — `orai setup`이 관리한다. 프로젝트 규칙은 AGENTS.md나 역할 지침에 둔다. -->

# 도구 상태 확인과 복구

역할이 없어도 쓴다.

## 확인

| 할 일 | 명령 |
|---|---|
| 전체 진단 | `orai doctor`. 출력의 "Next steps"가 남은 조치다 |
| 실제 검색까지 확인 | `orai doctor --deep` |
| 기계가 읽을 출력 | `orai doctor --json` |
| 역할별 실행 여부, 적용 중인 설정, 받지 않은 메시지 수 | `orai status` |
| shelf 서버만 확인 | `orai shelf check` |

- `not-checked`는 확인하지 못했다는 뜻이다. 문제가 있다는 근거도, 없다는 근거도 아니다.
- 샌드박스의 접근 거부와 도구 실패는 서버가 꺼졌다는 근거가 아니다. 샌드박스 밖에서 다시 확인한다.

## 복구

| 상황 | 명령 |
|---|---|
| 재부팅 뒤 shelf 서버가 꺼져 있다 | `orai shelf recover` |
| 문서를 고쳤다 | `orai shelf sync` |
| `orai.toml`의 `collections`를 바꿨다 | `orai shelf stop && orai shelf refresh` |
| `orai.toml`의 `context`를 바꿨다 | `orai shelf recover` |
| 생성 파일이나 설정이 어긋났다 | `orai setup` (바꿀 내용을 먼저 보려면 `orai setup --dry-run`) |
| 저장된 대화의 재개가 거부된다 | 사용자가 `orai <역할> --fresh`로 새 대화를 시작한다 |

- 그 밖의 조치는 `orai doctor`의 "Next steps"를 따른다.
- 명령별 항목은 `orai <명령> --help`에 있다.

## 커밋하지 않는 파일

`.orai/`(로컬 상태), `.agent-mail/`(메일함), `.agents/roles/`(역할 지침), `orai.local.toml`(이 컴퓨터의 설정). `.gitignore`가 제외한다.
