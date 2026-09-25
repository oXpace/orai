# 아키텍처

이 문서는 Orai의 책임 경계, 식별·상태 모델, 데이터 흐름을 소유한다. 명령 사용법과 장애 대응은 [운영 안내](operations.md), 지원 버전은 [호환성](compatibility.md), 추출 이력은 [이력](history.md)이 소유한다.

## 정의와 경계

Orai는 서로 다른 코딩 에이전트 CLI(Codex, Claude Code)를 **역할 세션**으로 실행하는 로컬 런처다. 세션끼리는 AMQ 메시지로 협업한다. Orai가 제공하는 것은 다섯 가지다.

1. 역할 세션 실행과 **정확한 대화 복구**
2. 새 메시지 **알림** 전달 (알림만 하고 메시지를 대신 소비하지 않는다)
3. 공통 **메시지 액션** (`inbox`, `send`, `reply`)
4. 프로젝트 탐색 도구(프로젝트 wiki, CodeGraph)의 설정·진단·복구
5. 위 네 가지의 **진단** (`doctor`)

소유권은 아래처럼 나뉜다. Orai는 다른 쪽의 책임을 다시 구현하지 않는다.

| 대상 | 소유자 | Orai의 역할 |
|---|---|---|
| 큐, 배달, receipt, thread | AMQ | `amq` CLI 호출, 신원 환경 구성 |
| 대화 내용, transcript, 모델 실행 | Codex / Claude | 정확한 세션 ID로 시작·재개 |
| 역할 조직, 업무 규칙, 승인, 이슈 트래커 정책 | 소비 프로젝트 | `orai.toml`과 역할 지침 경로를 읽어 전달만 함 |
| 문서 색인·검색, 코드 그래프 | QMD(wiki 엔진) / CodeGraph | 프로젝트별 격리 설정, 상태 진단, 명시적 복구 |

범위 밖: 자체 범용 MCP 서버, GUI, 클라우드 스케줄러, 플러그인 프레임워크. CLI와 진단이 완성되기 전에는 추가하지 않는다.

## 세 가지 위치

| 개념 | 정의 | 결정 방법 |
|---|---|---|
| 설치 위치 | `orai` 패키지가 설치된 곳 | `sys.executable`과 `python -m orai`. 소스 checkout 경로를 추정하지 않는다 |
| 프로젝트 루트 | `orai.toml`과 로컬 상태를 소유하는 main checkout | `--project` → 없으면 cwd에서 가장 가까운 `orai.toml`. Git worktree 안이면 main worktree에 `orai.toml`이 있을 때 main을 루트로 삼는다 |
| 역할 worktree | 역할 CLI가 작업하는 디렉터리 | `roles.<name>.worktree` (루트 기준 상대경로. `../repo-dev` 같은 형제 worktree도 가능) |

`orai.toml`이 없는 디렉터리에서는 프로젝트를 추측하지 않고 오류로 끝낸다. 역할이 있는 프로젝트는 루트에 `.amqrc`가 있어야 한다. 이 파일이 없으면 AMQ가 Git 밖에서 전역 `~/.amqrc`를 쓸 수 있고, 그러면 서로 다른 프로젝트가 같은 세션 mailbox를 공유하게 된다. `.amqrc`는 `orai setup`이 AMQ(`amq coop init`)를 통해 만든다. 역할 세션 안에서는 `ORAI_PROJECT`가 신원에 묶여 있으므로 다른 `--project`로 메시지 명령을 실행할 수 없다.

## 프로젝트 ID와 이동·복사 정책

프로젝트 ID는 `<이름 slug>-<루트 절대경로 sha256 앞 10자리>`다. 이 ID로 wiki의 QMD index 이름과 기본 포트를 정한다. 같은 저장소의 worktree는 모두 main checkout으로 해석되므로 ID가 같다. 서로 다른 경로에 있는 프로젝트는 ID가 다르다.

- **복사**: 복사본은 경로가 다르므로 새 ID를 받는다. 원본의 메일·포트·wiki 서버와 충돌하지 않는다. 함께 복사된 `.orai/project.json`의 `root`가 현재 경로와 다르면 `doctor`가 degraded로 보고한다.
- **이동**: 이동도 복사와 같이 새 ID가 된다. 저장된 세션은 기록된 `cwd`와 달라져 재개가 거부된다. `orai setup`으로 로컬 상태를 다시 연결한 뒤 `--fresh`로 새 대화를 시작한다. 이전 대화는 provider에 그대로 남는다.

## 설정과 로컬 상태

공유 설정(`orai.toml`, 커밋 대상)에는 이식 가능한 사실만 둔다. schema 버전, AMQ 세션 이름, 역할(provider, 상대경로, 선택적 model·effort·branch), 연동 설정이 여기에 해당한다. 모델과 effort는 코어에 기본값이 없다. 값이 없으면 각 provider의 기본값을 쓴다. 모르는 키, 절대경로, 루트를 벗어나는 지침 경로, 예약어 역할명(`init`, `run`, `user` 등 CLI 명령과 사용자 handle)은 거부한다.

로컬 상태(`.orai/`, 0700, 커밋 제외) 구조는 다음과 같다.

```
.orai/
  project.json              root, 초기화 버전·시각 (이동·복사 감지)
  roles/<role>.json         provider, cwd, nonce, status, session_id, transcript, captured_at
  roles/<role>.lock         역할당 하나의 실행 (flock; 자식 프로세스에 상속)
  roles/<role>.delivery.json / .channel.json   알림 준비 상태 (run nonce 단위)
  history/                  --fresh로 교체된 이전 상태
  backups/<stamp>/          init이 수정한 파일의 원본
  wiki/                     이 프로젝트 전용 wiki(QMD) 설정·DB
```

모든 상태 파일은 임시 파일에 쓴 뒤 `os.replace`로 원자적으로 교체하고, 권한은 0600으로 만든다.

## 세션 수명주기

```
orai run <role>
  ├─ 설정·worktree·branch 검증, 저장 상태 검증(provider·cwd·정확한 transcript)
  ├─ mail_env: 프로젝트 .amqrc 필수, 호출 셸의 AM_*·AMQ_GLOBAL_ROOT·ORAI_* 제거 → amq env로 base 해석
  ├─ role lock 획득 → mailbox 준비 → state{nonce, status=starting}
  ├─ amq coop exec --no-wake --no-init --named=false --root <session root> --me <role> <provider…>
  │     provider는 프로세스 인자로만 설정한다(전역 설정 불변)
  │       - SessionStart hook: python -m orai --project <root> _capture
  │       - Codex:  resume <UUID> | 새 대화, -c hooks/mcp_servers
  │       - Claude: --session-id <새 UUID> | --resume <UUID>, --settings, --mcp-config(orai 채널 + wiki)
  ├─ (Codex) notifier 스레드: codex queue --thread <UUID>
  └─ 종료: status=stopped, lock 해제
```

**신원 캡처.** provider의 SessionStart hook이 `_capture`를 호출한다. `_capture`는 환경변수의 run nonce가 상태 파일의 nonce와 같을 때만 세션 ID·transcript를 기록한다. 이전 실행이 남긴 낡은 hook은 nonce가 달라 거부되고, 신원을 덮어쓰지 못한다. cwd가 역할 worktree와 다르거나 이미 기록된 ID와 다른 ID가 오면 역시 거부한다. hook 출력은 역할·기준 경로 한 줄(300자 미만)만 복원하며 지침 전체를 다시 주입하지 않는다.

**정확한 재개.** 기본 동작은 캡처된 UUID로 재개하는 것이다. provider, cwd, 기록된 transcript 파일이 모두 일치해야 한다. "가장 최근 대화"를 추측하거나 writer lock을 지워 복구하지 않는다. 새 대화는 `--fresh`로만 시작하며, 이때 이전 상태는 `history/`로 옮긴다.

**부트스트랩 분리.** 새 세션 시작, 재개 안내, compact hook, 메시지 알림은 서로 다른 경로다. 새 세션만 역할 지침을 읽으라는 시작 프롬프트를 받는다. 재개는 요약을 이어받고 변경된 원문만 확인한다. 알림은 메시지 ID만 전달한다.

## 알림

알림 문구는 `오라이: 새 메시지\nIDs: <id,…>` 하나로 고정한다. notifier는 `amq list --new --json`만 호출하므로 메시지 소비와 receipt는 역할이 `orai msg inbox <ID>`를 실행할 때만 생긴다. 알림 성공, 큐 적재, 소비, 업무 완료는 모두 다른 상태다.

| provider | 경로 | 준비 조건 |
|---|---|---|
| Codex | 런처 안의 스레드가 `codex queue --thread <UUID>` 호출 | 같은 nonce로 신원 캡처 완료 |
| Claude | `python -m orai.providers.claude_channel` (stdio MCP, `claude/channel` capability) | initialize + `channel_ready` 도구 호출. `channel_ready`는 Orai가 제공하는 MCP 도구이며 Claude의 API 이름이 아니다 |

두 경로 모두 연결 중에 이미 알린 ID는 다시 알리지 않는다. 전달이 실패하면 다음 주기에 재시도하고, 재연결하면 남아 있는 ID를 다시 알린다.

## 탐색 도구 연동

코어(세션·메시지)는 연동 없이도 동작한다. 연동이 실패하면 숨기지 않고 보고하며, 에이전트는 `rg`와 원문 읽기로 작업을 이어간다.

**프로젝트 wiki (엔진 QMD).** 사용자와 에이전트는 `wiki`라는 이름만 본다(`orai wiki …`, `[integrations.wiki]`, MCP 서버 `wiki-<slug>`, 진단 항목 `wiki.*`). 엔진은 `integrations.wiki.engine`으로 지정하며 현재는 `qmd`만 지원한다. 프로젝트마다 index 이름 `orai-<project id>`를 쓴다. QMD 2.8.3 소스를 확인한 결과, `--index <name>`을 주면 daemon PID/로그 파일이 `~/.cache/qmd/mcp-<name>.pid|log`로 분리되고 설정 파일도 `<name>.yml`이 된다. 여기에 `QMD_CONFIG_DIR`과 `INDEX_PATH`로 설정과 DB를 `.orai/wiki/` 안에 둔다. 모델 캐시는 공유하므로 프로젝트마다 모델을 다시 받지 않는다. 서버는 `127.0.0.1`에만 열고, 포트는 설정값이 없으면 ID에서 유도한다(18200–18999). 이미 열린 포트는 MCP `status`의 컬렉션 절대경로가 이 프로젝트와 같을 때만 재사용한다. 중지는 index 범위의 `qmd --index <ours> mcp stop`만 쓴다. 역할 실행 시에는 MCP 서버 `wiki-<slug>`를 프로세스 인자로 주입하므로 전역·worktree 설정 파일을 바꾸지 않는다.

**CodeGraph.** 설치, provider MCP 등록, 프로젝트 색인은 사용자가 따로 진행하는 단계다. Orai는 `codegraph status --json`과 (deep 모드에서) 알려진 심볼 조회로 상태만 진단한다. 전역 등록과 중복될 수 있어 MCP를 자동으로 주입하지 않는다.

## 진단 모델

`doctor`는 읽기 전용이다. 구성요소마다 `component`, `status`, `reason`, `next_action`, `checked_at`을 JSON으로 낸다.

| status | 의미 |
|---|---|
| healthy | 확인한 범위에서 정상 |
| degraded | 동작하지만 일부 기능이나 신뢰도가 떨어짐 |
| not-ready | 설정은 됐지만 아직 준비 전 (색인 없음·비어 있음, 의미 검색 미실행 등) |
| blocked | 이 구성요소를 사용할 수 없음 |
| not-configured | 설정하지 않음 |
| not-checked | 이 모드에서 일부러 실행하지 않음 (예: `--deep` 없이 의미 검색). 전체 상태에 영향 없음 |
| unsupported | 설치된 버전에 필요한 기능이 없음 |

wiki 진단은 원인별로 나눈다. 연결 거부, 샌드박스 접근 거부, 다른 프로젝트 서버, MCP handshake 실패, 빈 색인, embedding 미완료, vector 검색 실패, lex+vec·본문 조회 실패가 각각 다른 항목으로 보고된다. 기본 doctor는 모델을 불러오지 않으므로 의미 검색 항목을 not-checked로 표시한다. not-checked는 검증했다는 뜻이 아니지만, 문제로 세지도 않는다. 의미 검색까지 확인하려면 `--deep`을 쓴다. 한 환경(예: 샌드박스 밖)에서 성공했다고 다른 환경에서도 성공했다고 확대하지 않는다.

종료 코드: 0 healthy / 1 degraded(또는 코어가 not-configured) / 2 사용법·설정 오류 / 3 코어 blocked. 메시지 명령은 AMQ의 종료 코드와 진단 출력을 그대로 전달한다.

## 모듈 지도

| 모듈 | 책임 |
|---|---|
| `cli.py` | 인자 파싱, `orai <role>` 별칭, 출력, 종료 코드 |
| `project.py` | 프로젝트 루트·worktree 해석, ID, 이동·복사 감지 |
| `config.py` | `orai.toml` schema 검증 |
| `state.py` | 원자적 저장, 역할 lock, 이전 상태 이력 |
| `mail.py` | AMQ 환경·신원, 메시지 액션 |
| `runtime.py` | 실행·캡처·재개 검증, status, 프로젝트 진단 |
| `providers/` | Codex·Claude 인자, Codex queue notifier, Claude MCP channel |
| `integrations/` | wiki 엔진(QMD) 수명주기·진단, CodeGraph 진단, 역할별 MCP 주입 |
| `doctor.py` | 진단 상태 모델, 도구 capability·로그인 검사 |
| `scaffold.py`, `templates/` | `orai setup`의 파일 계획·적용과 배포 템플릿 |
| `bootstrap.py` | `orai setup` 전체 흐름: 파일 → wiki → 코드 그래프 → 진단 |
