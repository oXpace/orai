# 이력

이 문서는 Orai의 추출 원천, 배경, 이미 알려진 장애, 원천에서 바뀐 내용, 검증 기록을 소유한다. 현재 설계는 [아키텍처](architecture.md)가 정본이다.

## 추출 원천

- 저장소: `oXpace/pockets` (로컬 `/Users/jason/Developments/Pockets`, 읽기 전용 참고)
- 기준 커밋: `b91019eb7f168d955bc5dd28d7b6b0d62fdcec04` (PR #381 병합, 2026-09-25)
- 원문은 `git show b91019e:<경로>`로 읽었다. 당시 worktree 상태와 같다고 가정하지 않았다.
- 원천은 저장소 작성자 소유 코드다. Orai의 라이선스는 아직 정하지 않았다([배포 계획](release.md)).

## 배경 (Pockets 커밋)

| 커밋 | 날짜 | 내용 |
|---|---|---|
| `5005c8a` | 2026-07-25 | desk 런처가 역할 문서와 AMQ 환경을 Codex/Claude 실행에 연결했다. 이때 역할과 큐 설정은 프로젝트에 결합돼 있었다 |
| `07ae35a` | 2026-09-12 | Telegraphy에서 Orai로 전환. AMQ co-op, provider별 SessionStart hook, Codex queue, Claude 로컬 MCP channel, 정확한 세션 복구, QMD 문서 탐색을 도입했다 |
| `0397c6a` | 2026-09-19 | `qmd-setup`의 init/recover/refresh/check와 회귀 테스트. 설정·색인을 보존하면서 MCP 프로젝트 식별·검색·본문 조회를 검증했다 |
| `2a231ea` | 2026-09-20 (원 작성) | 시작·복구·압축에서 과도한 지침 재주입과 반복 알림을 줄였다 |
| `f55f266` | 2026-09-24 | inbox/send/reply를 CLI로 통합. 알림은 짧은 문구와 메시지 ID만 전달한다 |
| `e60ded6` | 2026-09-24 | 본문 없는 대화형 stdin에서 무한히 기다리던 문제를 차단했다 |
| `58abbc8` | 2026-09-25 | PM·STAFF 두 상주 역할과 PM 전용 verifier로 전환. 프로젝트 프리셋의 사례일 뿐이며 코어의 역할 제한이 아니다 |
| `72b4088` | 2026-09-25 | `setup.NOTICE`를 바꾸면서 `scripts/desk`를 갱신하지 않아 설치 dry-run이 실패한 회귀를 수정했다. 임시 fixture 테스트로는 배포된 파일 사이의 불일치를 잡지 못했다 |

기준 시점 Pockets에서 Orai 테스트 34건, 머지 판정 fixture 68건, push-code gate가 통과했다. 프로젝트 verifier의 실제 호출과 두 역할 라이브 파일럿은 검증 완료로 기록되지 않았다.

## 이미 알려진 장애

- **QMD 다운을 doctor가 놓침.** `localhost:8181` 연결이 거부되는 상태에서 `orai doctor`가 성공을 반환했다. doctor가 QMD를 아예 검사하지 않았기 때문이다. `qmd-setup recover` 후 MCP 프로젝트 식별, 벡터·키워드 검색, 본문 조회는 성공했다. 서버가 내려간 근본 원인은 확인되지 않았고, 자동 상시 복구 기능도 없었다.
  - Orai의 대응: doctor가 연결 거부, 접근 거부, 다른 index, handshake, 빈 색인, embedding, 검색 실패를 원인별로 보고한다. 설정된 연동이 blocked이면 종료 코드가 0이 아니다. 복구는 명시적 `orai wiki recover`로 한다. 근본 원인과 상시 supervisor는 미해결로 남긴다.
- **QMD daemon PID 공유.** Pockets는 `INDEX_PATH`만 바꿨고 index 이름은 기본값이었다. 그래서 모든 프로젝트의 daemon이 `~/.cache/qmd/mcp.pid`를 공유했다(QMD 2.8.3 소스로 확인). `qmd mcp stop`을 실행하면 다른 프로젝트 서버가 멈출 수 있는 구조였다.
  - Orai의 대응: `--index orai-<id>`로 PID·로그·설정 파일 이름을 분리한다.

## 변환 내역

0.1.0은 Python으로 옮겼고, 0.2.0에서 Go로 다시 옮겼다(아래 [Go 전환](#go-전환-020)). 표의 Orai 열은 현재(Go) 위치다.

| 원천 (`b91019e`) | Orai | 바꾼 점과 이유 |
|---|---|---|
| `scripts/orai` | `cmd/orai`, `internal/cli` | 설치 위치를 루트로 추정하지 않는다. `--project`, cwd 탐색, `orai <role>` 별칭, `orai msg`로 메시지 명령 묶음 |
| `scripts/lib/orai_runtime.py` | `internal/{config,project,state,runtime,providers}` | `ROOT=스크립트 위치` → 프로젝트 루트 해석. 고정 pm/staff → 설정의 역할 목록과 provider 분기. "PM은 trunk" → 역할별 선택적 `branch`. 고정 세션 `orai` → `session` 설정. 역할명 기반 channel 분기 → provider 기반. `.agents/orai.json` → `orai.toml`(schema 1, 상대경로, 알 수 없는 키 거부). 모델·effort 선택화. 호출 셸의 `AM_*`·`AMQ_GLOBAL_ROOT`·`ORAI_*` 차단. 역할 세션 안에서 중첩 실행 거부. `amq coop exec` 대신 provider를 직접 실행 |
| AMQ 호출(`amq env/list/drain/read/send/reply/init`) | `internal/mail` | AMQ schema 1과 같은 디스크 형식의 자체 메일함. 2초 폴링 대신 파일 변경 이벤트(kqueue/inotify) + 보조 주기 |
| `scripts/lib/orai_channel.py` | `internal/channel` (`orai _channel`) | 고정 역할 검사 → 이름 형식 검사. 메일함 변경 이벤트로 알림 |
| `scripts/tests/test_orai_*.py` | `internal/{runtime,providers,channel,cli,mail}/*_test.go` | 계약을 그대로 유지하고 역할명을 일반화(`lead`/`dev`/`reviewer-2`). 실제 AMQ와의 양방향 호환 테스트 추가 |
| `scripts/qmd-setup`, `scripts/test-qmd-setup.py` | `internal/wiki`, `orai wiki …` | 8181·`docs`·한국어 질의 고정 → 프로젝트별 포트·컬렉션·smoke 설정. `--index` 격리. `stop` 추가. `--install`(전역 npm 설치) 제거. 검증 단계를 doctor 진단 항목과 공유 |
| `scripts/setup-orai.py`, `test_orai_setup.py` | `internal/{scaffold,setup}`, `orai setup` | 사전 충돌 검사, dry-run/apply, 원자적 쓰기, 백업, 멱등성을 참고했다. Telegraphy 마이그레이션은 가져오지 않았다. AGENTS/.gitignore는 관리 블록만 바꾼다 |
| `scripts/orai.md`, `.agents/skills/orai/**` | `docs/operations.md`, `internal/scaffold/templates/skill.md` | CLI 운영 문서와 에이전트용 메시지 액션을 분리했다. Pockets 고유 경로·Linear·TASKS 참조는 제거했다 |
| `.agents/orai.json`, `PM.md`, `STAFF.md` | `internal/scaffold/templates/{pm-staff.toml,pm.md,staff.md}` (`--preset pm-staff`) | 모델과 effort를 고정하지 않는 선택형 예제로 일반화했다 |

### 가져오지 않은 것

- 실제 `.orai` 상태·UUID·nonce·잠금, `.agent-mail` 큐·receipt·transcript, QMD/CodeGraph DB, 모델 파일, 개인 전역 설정·토큰, TASKS/Linear 데이터
- Pockets의 merge gate, 모바일 빌드, 제품 AC, JVM·iOS·Android 검증 체계
- Senior 역할과 `.codex/agents/verifier.toml`(Pockets의 모바일 검증에 특화됨). 필요하면 프로젝트 프리셋으로 다시 설계한다
- Pockets `pyproject.toml`(이름이 `pockets`인 초기 뼈대). Orai 패키지는 새로 정의했다(0.2.0부터 Go 모듈 `github.com/oXpace/orai`)
- 권장 구조의 `examples/pm-staff/`: 예제를 패키지 밖에 두면 `setup`이 배포하는 템플릿과 어긋날 수 있다. 그래서 바이너리에 내장한 템플릿과 `--preset pm-staff`로 단일화했다

## Go 전환 (0.2.0)

2026-09-25, 사용자 결정: 장기적으로 유리한 쪽, 특히 성능을 기준으로 Python에서 Go로 옮겼다. 같은 날 AMQ 의존도 자체 메일함으로 바꿨다.

- **이유**: 단일 실행 파일 배포(런타임 불필요), 빠른 시작, 파일 변경 이벤트 기반 알림, AMQ(Go, MIT)의 메일함 형식을 그대로 따를 수 있다는 점. Rust는 이 작업량에서 성능 차이가 드러나지 않고, 같은 생태계(AMQ, amq-squad, orca-cli)가 Go라 Go를 골랐다.
- **측정** (이 호스트, 중앙값): `--version` Python 70 ms → Go 6 ms, `status` 105 → 33 ms, `doctor` 336 → 207 ms(외부 도구 호출이 대부분). 알림 대기는 2초 폴링에서 이벤트 즉시 감지로 바뀌었다.
- **방식**: Python 테스트를 계약 명세로 삼아 Go 테스트로 옮겼다. 동작은 유지하고, 동등성을 확인한 뒤 Python 구현을 지웠다. 0.1.0 Release(Python)는 그대로 남겼다.
- **AMQ 대체**: Orai가 쓰던 AMQ 기능(메일함, 메시지 형식, 전송·답장·목록·수신·읽기, receipt)을 `internal/mail`로 구현했다. 실제 AMQ 0.80.1과 서로 보내고 받고 답장하는 교차 테스트를 둔다. 이 테스트가 AMQ가 요구하는 `dlq/` 폴더 누락을 잡아냈다. AMQ의 원격 중계, 깨우기, 실행기, swarm 등은 쓰지 않아 가져오지 않았다.
- **바뀐 점**: `orai setup`이 `.amqrc` 대신 메일함을 직접 만든다. Claude 채널은 `orai _channel`, hook은 설치된 `orai` 바이너리를 부른다. 설치 명령은 `mise use github:oXpace/orai@<버전>`이다.

## 사용성 개선 (0.3.0)

2026-10-03, 다른 환경에서 실제로 써 본 사용자가 세 가지를 지적했다: 초기 역할이 `pm-staff`로 고정돼 있다, wiki(QMD) 설정 안내가 부족하다, `doctor`가 친절하지 않다. 도구가 하나도 없는 PATH에서 `setup`과 `doctor`를 재현해 원인을 확인하고 고쳤다.

- **역할 선택**: `orai setup --role 이름=provider[:작업폴더]`를 추가했다. 새 프로젝트에서는 `orai.toml`을 그 역할로 만들고, 기존 프로젝트에서는 역할 표만 덧붙인다(백업 후). `pm-staff` 프리셋은 `--role pm=codex --role staff=claude`의 줄임이 됐고, 역할 없이 준비한 프로젝트에 나중에 적용해도 역할이 추가된다. 프리셋별 설정 템플릿 두 개를 하나(`config.toml`)로 합쳤다.
- **안내가 가리키는 곳**: 조치 문구가 `docs/compatibility.md`처럼 Orai 저장소에만 있는 문서를 가리키고 있었다. 설치 명령과 전체 URL로 바꿨다.
- **`doctor` 출력**: 기본 출력을 JSON에서 사람이 읽는 형식(구성요소별 한 줄, 상태, 번호 붙은 Next steps, Optional)으로 바꾸고 JSON은 `--json`으로 옮겼다. 종료 코드와 JSON 필드는 그대로다. `setup`의 요약도 같은 형식을 쓴다. 이유 문구에서 절대 경로를 프로젝트 기준 경로로 줄였고, 역할 worktree 조치는 첫 커밋 여부에 따라 달라진다. **호환성 영향**: `orai doctor`의 출력을 JSON으로 읽던 스크립트는 `--json`을 붙여야 한다.
- **조치 없는 문제**: 조치 문구가 비어 있는 문제 항목(예: npm으로 설치했지만 `node`가 PATH에 없어 실행되지 않는 CodeGraph)은 Next steps와 "N to fix" 개수에서 빠졌다. 이제 모든 문제 항목이 조치를 갖고, 개수도 문제 항목 수와 같다. 선언한 역할 지침 파일이 없으면 `doctor`가 알리고 `setup`이 틀을 만든다(역할 이름을 손으로 바꾼 경우).
- **저장소에 넣는 것과 넣지 않는 것**: `setup`이 `.agents/.gitignore`에 `/roles/`를 넣어 역할 지침을 컴퓨터마다 두도록 했다(사용자 요청). `AGENTS.md`의 Orai 블록은 Orai가 무엇인지와 역할·메시지 사용법, 커밋하지 않는 경로를 설명하도록 늘렸다. 이 과정에서 `TestThisRepositoryIsSetupCurrent`가 실패 대신 skip하고 있어 생성 파일 누락을 잡지 못하던 것을 발견해 실패하도록 고쳤다.
- **시작 프롬프트**: 한 문단에 규칙까지 섞여 있던 시작 프롬프트를 기본 사항만 담은 구조(프로젝트·역할·작업 폴더, 문서 세 개의 경로, 시작 순서)로 바꿨다(사용자 요청). 프롬프트에 있던 규칙(프로젝트 폴더의 지침이 기준, 세션 ID·메일 설정 변경 금지)은 `AGENTS.md`의 Orai 블록으로 옮겼다. 시작 단계 중 모든 역할에 공통인 것(문서 읽기, 메시지 확인, 턴 종료)은 프롬프트에 두고, provider마다 다른 것(Claude의 `channel_ready`)만 역할을 추가할 때 역할 지침의 "세션 시작" 절로 써 넣는다. 프롬프트는 그 절이 있으면 가리키고, Claude 지침에 `channel_ready`가 없으면 직접 적는다. 프롬프트에서 절대 경로는 프로젝트 폴더 한 줄뿐이고, 문서 경로는 그 폴더 기준 상대 경로로 적는다. 세션을 다시 열 때는 문서 목록 없이 단계와 지침 경로 한 줄만 보낸다.
- **중복 처리**: Codex 역할이 이미 읽은 메시지의 알림을 다시 받아 같은 일을 반복한다는 보고가 있었다. 알림은 Codex 대기열에 들어가면 회수할 수 없는데, `orai msg inbox <ID>`가 이미 받은 메시지의 본문을 아무 표시 없이 다시 돌려주고 있었다(전체 수신 뒤 ID 수신으로 재현). 세션을 열 때는 시작 단계의 전체 수신과 대기 메시지 알림이 겹쳐 매번 이 조건이 생긴다. 이제 이미 받은 메시지는 본문 없이 `already_received`와 받은 시각만 돌려주고, `--again`으로만 다시 읽는다. 스킬에도 같은 규칙을 적었다. AMQ 0.80.1~0.85.0에는 이와 관련한 수정이 없다.
- **wiki 안내**: `orai wiki --help`(인자 없는 `orai wiki` 포함)에 준비 순서, 모델 다운로드 크기, 문서 갱신 방법, 설정 키를 넣었다. 생성되는 `orai.toml`과 `docs/README.md`에도 같은 내용을 적었다. 색인이 없는 프로젝트의 `orai wiki check`는 "connection refused" 대신 무엇을 실행할지 말한다. 색인을 만든 뒤 `collections`에 추가한 폴더는 `refresh`가 색인에 추가한다(이전에는 `.orai/wiki`의 설정 파일을 직접 고쳐야 했다).
- **함께 고친 버그**: `orai setup --help`가 도움말을 보여주는 대신 setup을 실행했다. 파서가 `--help`를 기록만 하고 아무도 확인하지 않았다. 이제 모든 명령에 `--help`가 있고, 종단 테스트가 도움말이 파일을 만들지 않는지 확인한다.

## 검증 기록

### 2026-10-03, 0.3.0 개발본 (macOS 27.0 arm64)

| 범위 | 결과 |
|---|---|
| `mise run check` (gofmt, go vet, race 검사기를 켠 전체 테스트, 종단 테스트) | 통과. Linux는 CI에서만 확인한다 |
| 메일함 양방향 호환 | 통과, AMQ 0.85.0 (0.80.1 이후 디스크 형식 변경 없음을 릴리스 노트와 변경 파일 목록으로도 확인) |
| 도구 진단 | 통과: Codex 0.160.0, Claude Code 2.1.288의 capability와 로그인 |
| wiki | 통과: QMD 2.8.3으로 이 저장소 문서 색인, `orai doctor --deep`의 vector·lex+vec·본문 조회 |
| 코드 그래프 | 통과: CodeGraph 1.6.0 색인과 smoke 심볼 조회 |
| 빈 폴더 흐름 | 통과: 도구가 없는 PATH와 도구가 모두 있는 PATH에서 `setup --role` → 커밋 → `git worktree add` → `doctor` healthy |
| 기존 저장소에 추가, 복제본에서 `setup` | 통과: 기존 `AGENTS.md`·`.gitignore` 보존, 브랜치·커밋 불변, 복제본에는 로컬 상태만 생성 |
| 시작 프롬프트 | `orai <역할> --dry-run` 출력으로만 확인. **실제 Codex·Claude 세션에서는 미검증** |
| 설치 | `mise use --pin github:oXpace/orai`가 최신 릴리스를 고정하는 것을 0.2.0으로 확인. 0.3.0 설치는 배포 후 확인 |

| 이미 받은 메시지의 재수신 표시 | 통과: 전체 수신 뒤 `orai msg inbox <ID>`가 본문 없이 `already_received`를 돌려준다(CLI와 테스트). 실제 Codex 세션에서 중복 처리가 사라지는지는 미검증 |

### 2026-09-25, 0.2.0 (Go, macOS 27.0 arm64)

| 범위 | 결과 |
|---|---|
| `mise run check` (gofmt, go vet, race 검사기를 켠 전체 테스트, 빌드한 바이너리의 종단 테스트) | 통과. 최상위 테스트 157개(하위 포함 375개), 3회 반복 모두 통과 |
| CI (macOS 15 + 실제 AMQ, Ubuntu 24.04, 4개 타깃 크로스 빌드) | 통과 |
| 메일함 AMQ 0.80.1 양방향 호환 | 통과 (Orai → `amq drain`·`reply`, `amq send` → Orai 수신·답장) |
| 이 저장소에서 Go 바이너리로 `orai doctor --deep` | 기존 wiki 서버(포트 18800)를 같은 프로젝트로 인식. vector·lex+vec·본문 조회 healthy, CodeGraph 심볼 조회 healthy |
| Release `0.2.0` → 빈 폴더에서 `mise use github:oXpace/orai@0.2.0` | 2.9초 만에 설치, `orai 0.2.0` |
| 이어서 `orai setup --preset pm-staff` | trunk 저장소, 파일, 메일함, wiki(문서 1개, 전용 포트), CodeGraph 색인 완료. 첫 커밋과 `git worktree add .worktrees/staff` 후 `orai doctor` healthy |
| 설치된 바이너리로 메시지 흐름 | Desktop `msg send` → staff worktree에서 역할 신원으로 `msg inbox` 수신. `amq send` → Orai 수신. staff worktree에서 `orai staff --dry-run`이 main 프로젝트와 메일함을 해석 |

### 2026-09-25, 0.1.0 (Python, macOS 27.0 arm64)

| 범위 | 결과 |
|---|---|
| `mise run check` (ruff, pytest 174건, wheel 설치 테스트 5건) | 통과. Python 3.14.7과 3.11.16 모두 |
| 실제 AMQ 0.80.1 임시 큐 통합 (send → peek → inbox → reply → thread, 파일·stdin 본문, ID 직접 수신) | 통과 |
| 호스트 도구 capability와 로그인 (`orai doctor`) | AMQ 0.80.1, Codex 0.157.0, Claude Code 2.1.282 healthy, 두 계정 로그인 확인 |
| 이 저장소 문서의 실제 QMD 2.8.3 (`orai wiki init`, `orai doctor --deep`) | 통과. index `orai-orai-f9afb187da`, 포트 18800, 문서 5개(24 chunks, embed 18초). vector-only 검색이 smoke 문서(`docs/architecture.md`)를 1위로 반환. lex+vec 검색과 본문 조회 통과. 기존 8181 서버(PID 3296)와 `~/.cache/qmd/mcp.pid`는 변경 없음 |
| QMD 장애 재현 (`orai wiki stop` 후 `orai doctor`) | `wiki.server` blocked(connection refused), 전체 degraded, exit 1. `orai wiki recover` 후 다시 healthy |
| 실제 CodeGraph 1.5.0 (`codegraph init`, `orai doctor --deep`) | 통과. 29개 파일, 616 노드, 1,628 엣지. smoke 심볼 `validate_saved`가 `src/orai/runtime.py`에서 조회됨. `init`은 `.codegraph/`만 만들었고 지침 파일은 바꾸지 않음 |

임베딩 모델 기록: `hf:Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-Q8_0.gguf`, 파일 639,150,592 bytes, sha256 `06507c7b42688469c4e7298b0a1e16deff06caf291cf0a5b278c308249c3e439`(2026-09-12 다운로드, HF revision은 기록되지 않음). 검색 후 서버 RSS는 약 1.0 GB였다. 문서 5개에 대한 smoke 질의 1건의 결과이므로 한국어 recall 품질을 보장하지 않는다.

발견하고 고친 문제:
- QMD 2.8.3의 검색 결과 `file`은 `docs/<경로>` 형식이다(`qmd://` 접두사 없음). smoke 문서를 찾고도 불일치로 판정했기에 두 형식을 정규화하도록 고쳤다. 기존 테스트는 기대값을 같은 함수에서 만들어 이 문제를 놓쳤으므로 리터럴 회귀 테스트를 추가했다.
- `Path.mkdir(parents=True, mode=0o700)`은 마지막 디렉터리에만 mode를 적용해 `.orai/`가 0755로 만들어졌다. 모든 상위 디렉터리를 0700으로 만들도록 고쳤다.
- Git 밖 프로젝트는 AMQ가 전역 `~/.amqrc`를 쓸 수 있어 프로젝트 사이에 mailbox가 섞일 수 있었다. 역할이 있는 프로젝트는 루트의 `.amqrc`를 필수로 하고, `orai init`이 `amq coop init`으로 만들도록 했다.

독립 코드 검토(같은 날)에서 확인되어 고친 결함. 각 항목에 회귀 테스트를 추가했다.
- 모노레포 안의 하위 프로젝트(`mono/sub/orai.toml`)가 상위 `mono`로 해석됐다. 이제 linked worktree일 때만 main checkout의 같은 상대 위치로 옮긴다.
- 저장소 하위 디렉터리에 있는 프로젝트가 `worktree = "."` 역할을 실행하지 못했다. worktree 최상위 검사를 별도 worktree에만 적용한다.
- `init --apply`가 실행하기 전에 모든 항목을 "Applied"로 출력했다. 이제 성공한 항목만 출력한다.
- git이 없으면 `doctor`가 보고서 대신 예외로 끝났다.
- provider 실행이 시작되지 못하면 상태가 `starting`으로 남아 다음 재개가 막혔다. 이제 이전 상태를 되돌린다.
- doctor가 실행에 필요한 Claude(`--name`, `--effort`, `--add-dir`)와 Codex(`--add-dir`, `--config`) 플래그를 검사하지 않았다.

명령 구조 정리(같은 날, 사용자 요청):
- 기본 브랜치를 `trunk`로 했다. `setup`이 새로 만드는 저장소도 `trunk`를 쓴다(`--branch`로 변경 가능).
- 메시지 명령을 `orai msg inbox|send|reply`로 내렸다.
- QMD를 `wiki`로 감쌌다: `orai wiki …`, `[integrations.wiki]`(`engine = "qmd"`), MCP 서버 `wiki-<slug>`, 진단 항목 `wiki.*`, 상태 `.orai/wiki/`.
- `orai init`을 `orai setup`으로 합쳤다. 빈 폴더에서 저장소, 파일, AMQ 루트, docs 시작 페이지, wiki, 코드 그래프, 진단까지 한 번에 처리한다.
- 옛 명령(`orai inbox`, `orai qmd`, `orai init`)은 새 명령을 안내하고 종료 코드 2로 끝난다. 이름이 바뀌기 전의 관리 블록 표식도 새 표식으로 교체된다.
- 기본 doctor가 의미 검색을 실행하지 않아 항상 degraded가 되던 문제를 고쳤다. 해당 항목은 `not-checked`(검증 아님, 문제도 아님)로 표시한다.
- 검증: 임시 Git 원천에서 `uvx --from git+file://…@trunk orai setup`을 실행했다. 빈 폴더가 trunk 저장소가 됐고, 파일 생성, wiki 색인·서버(포트 18267, 문서 1개), CodeGraph 색인을 거쳐 진단이 healthy였다. `mise use pypi:…`는 로컬 원천을 지원하지 않아 GitHub 게시 후 검증한다.

공개 배포(같은 날): MIT(© oXpace)로 `github.com/oXpace/orai`를 공개하고 Release `0.1.0`을 만들었다. CI는 macOS 15·Ubuntu 24.04 × Python 3.11·3.14.7 네 조합 모두 통과했다(macOS에서는 실제 AMQ를 설치해 통합 테스트까지 실행). 빈 폴더에서 `mise use pypi:oXpace/orai@0.1.0`으로 설치했다. mise는 `uv tool install`로 GitHub Release 태그 소스를 설치해 `mise.toml`에 고정했다. 이어서 `orai setup --preset pm-staff`로 trunk 저장소, 파일, AMQ 루트, wiki(문서 1개, 전용 포트), CodeGraph 색인까지 완료했다. 남은 진단은 첫 실행 때 만들어지는 mailbox와 staff worktree 생성뿐이었고, staff worktree의 안내 문구를 `git worktree add`로 고쳤다.

### 미검증

- Codex↔Claude 실제 요청·회신, 알림 도착(Codex queue·Claude channel), 종료 후 동일 UUID 복구, 중복 실행 거부.
- 설치 버전의 SessionStart hook 전제: Claude 재개 시 `session_id` 유지, Codex payload의 `cwd`·`transcript_path`, `codex resume` 시 hook 실행. 틀리면 실패는 안전한 쪽(신원 덮어쓰기 없이 캡처 거부)이지만 해당 provider의 재개나 알림이 동작하지 않는다. [운영 안내](operations.md#실제-파일럿-계정호스트-준비-후) 절차로 수행한다.
- Linux에서의 실제 provider 실행, QMD CPU fallback, 재부팅 후 자동 복구(현재는 수동 `recover`).
