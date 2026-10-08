# 아키텍처

이 문서는 Orai의 책임 경계, 식별·상태 모델, 데이터 흐름을 소유한다. 명령 사용법과 장애 대응은 [운영 안내](operations.md), 지원 버전은 [호환성](compatibility.md), 추출 이력은 [이력](history.md)이 소유한다.

## 정의와 경계

Orai는 서로 다른 코딩 에이전트 CLI(Codex, Claude Code)를 **역할 세션**으로 실행하는 프로젝트 하네스다. 세션끼리는 프로젝트 메일함으로 메시지를 주고받으며 협업한다. Go로 작성한 단일 실행 파일이며, Orai가 제공하는 것은 여섯 가지다.

1. 역할 세션 실행과 **정확한 대화 복구**
2. 프로젝트 **메일함**(AMQ와 같은 디스크 형식)
3. 새 메시지 **알림** 전달 (알림만 하고 메시지를 대신 소비하지 않는다)
4. 공통 **메시지 액션** (`orai msg inbox|send|reply`)
5. 프로젝트 탐색 도구(shelf, CodeGraph)의 설정·진단·복구
6. 위 항목의 **진단** (`doctor`)

소유권은 아래처럼 나뉜다. Orai는 다른 쪽의 책임을 다시 구현하지 않는다.

| 대상 | 소유자 | Orai의 역할 |
|---|---|---|
| 메일함, 배달, receipt, thread | Orai (`internal/mail`) | AMQ schema 1과 같은 디스크 형식이라 `amq`도 같은 메일함을 읽고 쓴다. AMQ 설치는 필요 없다 |
| 대화 내용, transcript, 모델 실행 | Codex / Claude | 정확한 세션 ID로 시작·재개 |
| 역할 조직, 업무 규칙, 승인, 이슈 트래커 정책 | 소비 프로젝트 | `orai.toml`과 역할 지침 경로를 읽어 전달만 함 |
| 문서 색인·검색, 코드 그래프 | QMD(shelf 엔진) / CodeGraph | 프로젝트별 격리 설정, 상태 진단, 명시적 복구 |

범위 밖: 자체 범용 MCP 서버, GUI, 클라우드 스케줄러, 플러그인 프레임워크. CLI와 진단이 완성되기 전에는 추가하지 않는다.

## 세 가지 위치

| 개념 | 정의 | 결정 방법 |
|---|---|---|
| 설치 위치 | `orai` 실행 파일이 있는 곳 | 실행 중인 바이너리 경로(`os.Executable`). 소스 checkout 경로를 추정하지 않는다 |
| 프로젝트 루트 | `orai.toml`과 로컬 상태를 소유하는 main checkout | `--project` → 없으면 cwd에서 가장 가까운 `orai.toml`. Git worktree 안이면 main worktree에 `orai.toml`이 있을 때 main을 루트로 삼는다 |
| 역할 worktree | 역할 CLI가 작업하는 디렉터리 | `roles.<name>.worktree` (루트 기준 상대경로. `../repo-dev` 같은 형제 worktree도 가능) |

`orai.toml`이 없는 디렉터리에서는 프로젝트를 추측하지 않고 오류로 끝낸다. 메일함은 항상 `<프로젝트 루트>/.agent-mail/<session>`이며 프로젝트 루트에서만 결정된다. 호출 셸의 `AM_*`·`ORAI_*` 환경변수나 전역 설정이 다른 프로젝트의 메일함을 가리키게 할 수 없다. 역할 세션 안에서는 `ORAI_PROJECT`가 신원에 묶여 있으므로 다른 `--project`로 메시지 명령을 실행할 수 없다.

## 프로젝트 ID와 이동·복사 정책

프로젝트 ID는 `<이름 slug>-<루트 절대경로 sha256 앞 10자리>`다. 이 ID로 shelf의 QMD index 이름과 기본 포트를 정한다. 같은 저장소의 worktree는 모두 main checkout으로 해석되므로 ID가 같다. 서로 다른 경로에 있는 프로젝트는 ID가 다르다.

- **복사**: 복사본은 경로가 다르므로 새 ID를 받는다. 원본의 메일·포트·shelf 서버와 충돌하지 않는다. 함께 복사된 `.orai/project.json`의 `root`가 현재 경로와 다르면 `doctor`가 degraded로 보고한다.
- **이동**: 이동도 복사와 같이 새 ID가 된다. 저장된 세션은 기록된 `cwd`와 달라져 재개가 거부된다. `orai setup`으로 로컬 상태를 다시 연결한 뒤 `--fresh`로 새 대화를 시작한다. 이전 대화는 provider에 그대로 남는다.

## 설정과 로컬 상태

공유 설정(`orai.toml`, 커밋 대상)에는 이식 가능한 사실만 둔다. schema 버전, 메일함 세션 이름, 역할(provider, 상대경로, 선택적 model·effort·branch), 연동 설정이 여기에 해당한다. 모델과 effort는 코어에 기본값이 없다. 값이 없으면 각 provider의 기본값을 쓴다. 모르는 키, 절대경로, 루트를 벗어나는 지침 경로, 예약어 역할명(`setup`, `run`, `msg`, `shelf`, `user` 등 CLI 명령과 사용자 handle)은 거부한다. 현재 schema는 2다. schema 1은 문서 검색 표의 이름이 `[integrations.wiki]`였고, 그 파일도 그대로 읽는다. Orai는 `orai.toml` 본문을 고치지 않으므로 doctor가 고칠 두 줄을 알려 준다.

이 컴퓨터만의 설정은 `orai.toml` 옆의 `orai.local.toml`(커밋 제외, 없어도 된다)에 둔다. 역할 worktree에서 실행해도 프로젝트는 main checkout이므로 이 파일의 자리는 main checkout 한 곳이다. 읽는 순서는 다음과 같다.

1. `orai.toml`을 혼자서 검증한다. 그래서 로컬 파일이 없는 checkout은 언제나 동작하고, 로컬 파일이 공유 파일의 오류를 가리지 못한다.
2. 로컬 파일을 키 단위로 겹친다. 표는 같은 이름의 표와 합치고, 그 밖의 값은 공유 값을 대신한다.
3. 겹친 결과를 파일 하나를 읽을 때와 같은 규칙으로 검증한다. 여기서 난 오류는 로컬 파일의 것으로 보고한다.

로컬 파일은 `roles`와 `integrations` 아래의 모든 키를 정할 수 있다. `schema`, `name`, `session`은 정할 수 없다. 프로젝트 ID, 색인 이름, 메일함 경로가 여기서 나오므로 한 컴퓨터에서만 바뀌면 자기 색인과 메일함을 잃는다. 선언을 지우는 방법은 없다(TOML에 "없음"을 뜻하는 값이 없고, 임의의 표식을 만들지 않았다). 스키마에 목록 값이 없어 목록을 합치는 규칙도 없다.

로컬 파일이 정한 키는 설정과 함께 기록되어 `orai doctor`의 `project` 항목과 `orai status`가 보여 준다. 진단과 실행은 모두 겹친 결과를 기준으로 한다. 커밋되는 생성물(`.gitignore` 블록)만 공유 설정으로 계산한다. 그러지 않으면 사람마다 블록이 달라진다. 로컬 설정 때문에 더 무시해야 하는 경로(로컬 역할의 worktree 등)는 저장소의 `.git/info/exclude`에 같은 방식의 블록으로 넣는다.

로컬 상태(`.orai/`, 0700, 커밋 제외) 구조는 다음과 같다.

```
.orai/
  project.json              root, 초기화 버전·시각 (이동·복사 감지)
  roles/<role>.json         provider, cwd, nonce, status, session_id, transcript, captured_at
  roles/<role>.lock         역할당 하나의 실행 (flock; 자식 프로세스에 상속)
  roles/<role>.delivery.json / .channel.json   알림 준비 상태 (run nonce 단위)
  history/                  --fresh로 교체된 이전 상태
  backups/<stamp>/          setup이 수정한 파일의 원본
  shelf/                    이 프로젝트 전용 shelf(QMD) 설정·DB
.agent-mail/<session>/       프로젝트 메일함 (AMQ schema 1 호환, 0700/0600)
  meta/config.json          {"agents": [...], "created_utc", "version": 1}
  agents/<handle>/inbox/{tmp,new,cur}/<id>.md
  agents/<handle>/outbox/sent/<id>.md, dlq/{tmp,new,cur}/, receipts/<id>__<handle>__drained.json
```

모든 상태 파일은 임시 파일에 쓰고 fsync한 뒤 rename으로 원자적으로 교체하며, 권한은 0600으로 만든다. 메시지 배달은 `inbox/tmp`에 쓴 뒤 `link(2)`로 `inbox/new`에 게시하므로 기존 메시지를 덮어쓰지 않는다. 메시지 파일은 `---json` 머리말(schema, id, from, to, thread, subject, created, refs, priority, kind)과 본문으로 되어 있다. 1:1 thread 이름은 `p2p/<작은 handle>__<큰 handle>`이다. 답장은 원본의 thread를 잇고 refs에 원본 ID를 넣으며, question에는 answer, review_request에는 review_response kind를 붙인다. `inbox`(drain)와 `inbox <ID>`(read)는 메시지를 `cur/`로 옮기고 drained receipt를 남긴다. `inbox --peek`와 알림은 목록만 읽는다.

## 세션 수명주기

```
orai run <role>
  ├─ 설정·worktree·branch 검증, 저장 상태 검증(provider·cwd·정확한 transcript)
  ├─ role lock 획득 → 메일함 준비(없는 handle 추가) → state{nonce, status=starting}
  ├─ provider 직접 실행 (lock fd를 자식에 상속, SIGTERM·SIGHUP은 자식에 전달)
  │     환경: 호출 셸의 AM_*·AMQ_GLOBAL_ROOT·ORAI_* 제거 후 ORAI_ROLE/PROJECT/SESSION/RUN_NONCE/MAIL_ROOT,
  │           AMQ 호환을 위한 AM_ROOT/AM_ME/AM_SESSION 설정
  │     provider는 프로세스 인자로만 설정한다(전역 설정 불변)
  │       - SessionStart hook: <orai 바이너리> --project <root> _capture
  │       - Codex:  resume <UUID> | 새 대화, -c hooks/mcp_servers
  │       - Claude: --session-id <새 UUID> | --resume <UUID>, --settings, --mcp-config(orai 채널 + shelf)
  ├─ (Codex) notifier: 메일함 변경 이벤트(kqueue/inotify) + 2초 보조 주기 → codex queue --thread <UUID>
  └─ 종료: status=stopped, lock 해제
```

**신원 캡처.** provider의 SessionStart hook이 `_capture`를 호출한다. `_capture`는 환경변수의 run nonce가 상태 파일의 nonce와 같을 때만 세션 ID·transcript를 기록한다. 이전 실행이 남긴 낡은 hook은 nonce가 달라 거부되고, 신원을 덮어쓰지 못한다. cwd가 역할 worktree와 다르거나 이미 기록된 ID와 다른 ID가 오면 역시 거부한다. hook 출력은 역할·기준 경로 한 줄(300자 미만)만 복원하며 지침 전체를 다시 주입하지 않는다.

**정확한 재개.** 기본 동작은 캡처된 UUID로 재개하는 것이다. provider, cwd, 기록된 transcript 파일이 모두 일치해야 한다. "가장 최근 대화"를 추측하거나 writer lock을 지워 복구하지 않는다. 새 대화는 `--fresh`로만 시작하며, 이때 이전 상태는 `history/`로 옮긴다.

**부트스트랩 분리.** 새 세션 시작, 재개 안내, compact hook, 메시지 알림은 서로 다른 경로다. 알림은 메시지 ID만 전달한다.

시작 프롬프트는 기본 사항만 담는다. 프로젝트·역할·프로젝트 폴더, 읽을 문서 세 개의 경로(프로젝트 `AGENTS.md`, `orai.toml`의 `guide`가 가리키는 역할 지침, 메시지 스킬), 그리고 시작 단계다. 작업 규칙은 프롬프트가 아니라 그 문서들이 소유한다. 프로젝트 폴더만 루트의 절대 경로로 적고, 문서 경로는 모두 그 프로젝트 폴더 기준 상대 경로다(`AGENTS.md`, `.agents/roles/dev.md`). `orai.toml`에 적힌 값과 같고, 어느 폴더에서 일하는 역할이든 같은 경로를 받는다. 자기 worktree에서 일하는 역할이 worktree의 사본 대신 프로젝트 폴더의 문서를 읽어야 한다는 규칙은 `AGENTS.md`의 Orai 블록에 있다.

시작 단계는 모든 역할에 공통인 것과 일부 역할에만 필요한 것으로 나뉜다.

- **공통 (프롬프트에 적는다)**: 문서를 읽는다, `orai msg inbox`로 메시지를 확인한다, 처리할 것이 없으면 턴을 끝낸다.
- **역할별 (역할 지침의 "세션 시작" 절에 적는다)**: 현재는 Claude 역할의 `channel_ready` 호출뿐이다. `setup`이 Claude 역할을 추가할 때 이 절을 지침에 써 넣는다. Codex 역할의 지침에는 이 절이 없다. 프로젝트가 역할마다 세션을 열 때 할 일을 더 정하고 싶으면 이 절에 적는다.

프롬프트는 지침에 "세션 시작" 절이 있을 때만 "그 절을 따른다"는 단계를 넣는다. Claude 역할인데 지침에 `channel_ready`가 없으면(지침이 없거나, 이전 버전에서 만들었거나, 손으로 쓴 경우) 프롬프트가 그 호출을 직접 적는다. 이 호출이 빠지면 알림이 조용히 오지 않기 때문에 지침 내용에만 맡기지 않는다.

다시 연 세션은 문서 목록 없이 같은 단계("문서를 읽는다" 제외, 마지막은 "하던 작업을 이어간다")와 지침 경로 한 줄만 받는다. SessionStart hook은 역할과 문서 위치를 한 줄로 다시 알려 줄 뿐 문서를 다시 읽게 하지 않는다. `orai run <역할> --dry-run`으로 실제 프롬프트를 볼 수 있다.

## 알림

알림 문구는 `오라이: 새 메시지\nIDs: <id,…>` 하나로 고정한다. notifier와 채널은 `inbox/new` 목록만 읽으므로 메시지 소비와 receipt는 역할이 `orai msg inbox <ID>`를 실행할 때만 생긴다. 알림 성공, 큐 적재, 소비, 업무 완료는 모두 다른 상태다.

| provider | 경로 | 준비 조건 |
|---|---|---|
| Codex | 런처 안의 notifier가 `codex queue --thread <UUID>` 호출 | 같은 nonce로 신원 캡처 완료 |
| Claude | `orai _channel` (stdio MCP, `claude/channel` capability) | initialize + `channel_ready` 도구 호출. `channel_ready`는 Orai가 제공하는 MCP 도구이며 Claude의 API 이름이 아니다 |

두 경로 모두 새 메시지를 파일 변경 이벤트로 즉시 감지하고, 이벤트를 놓쳐도 2초 보조 주기로 확인한다. 연결 중에 이미 알린 ID는 다시 알리지 않는다. 전달이 실패하면 다음 주기에 재시도하고, 재연결하면 남아 있는 ID를 다시 알린다.

## 탐색 도구 연동

코어(세션·메시지)는 연동 없이도 동작한다. 연동이 실패하면 숨기지 않고 보고하며, 에이전트는 `rg`와 원문 읽기로 작업을 이어간다.

**shelf (문서 검색, 엔진 QMD).** shelf는 등록한 문서 폴더를 찾아볼 수 있게 하는 색인이다. 지식을 정리하거나 소유하지 않으며, 원본은 저장소의 파일이다. 사용자와 에이전트는 `shelf`라는 이름만 본다(`orai shelf …`, `[integrations.shelf]`, MCP 서버 `shelf`, 진단 항목 `shelf.*`). MCP 서버 이름은 모든 프로젝트에서 같다. 세션은 자기 프로젝트의 서버 하나에만 연결되므로 이름으로 프로젝트를 구분할 필요가 없고, 같은 이름이어야 지침과 도구 허용(`mcp__shelf__*`)을 프로젝트마다 다시 쓰지 않는다. 묶음(collection)은 폴더이거나 폴더의 일부(`{ path, pattern }`)이며, `orai.toml`이 선언이고 색인은 그 결과다. `init`과 `refresh`는 색인을 선언에 맞추고(추가, 다시 등록, 삭제), `recover`는 서버만 켜므로 차이를 알려 주기만 한다. 문서 내용이 바뀐 것은 묶음 선언과 무관하므로 `sync`가 서버를 멈추지 않고 색인과 임베딩만 갱신한다. 서버와 CLI가 같은 SQLite 파일을 쓰고, 서버는 다음 검색에서 갱신된 색인을 읽는다. 폴더별 설명(context, `[integrations.shelf.context]`)도 선언이 원천이다. 엔진은 설명을 묶음별로 `.orai/shelf/`의 자기 설정에 두는데 이것은 로컬 상태라 새 checkout에는 없다. 그래서 설명은 폴더 경로로 선언하고, Orai가 그 폴더를 포함하는 묶음과 그 폴더 안에 있는 묶음에 나눠 적용한다. 설명은 다시 색인할 필요가 없고 실행 중인 서버가 다음 검색부터 읽으므로 `init`, `refresh`, `recover`가 모두 적용한다. 표를 선언하지 않은 프로젝트의 설명은 건드리지 않는다. 검색 결과는 MCP 응답의 구조화된 부분에 실리는데 이를 모델에 넘기는지는 provider가 정한다. Orai는 그 사이에 끼지 않으므로, 지침은 어느 provider에서나 성립하는 "shelf로 찾고 원문은 파일로 읽는다"를 기준으로 한다. 엔진은 `integrations.shelf.engine`으로 지정하며 현재는 `qmd`만 지원한다. 프로젝트마다 index 이름 `orai-<project id>`를 쓴다. QMD 2.8.3 소스를 확인한 결과, `--index <name>`을 주면 daemon PID/로그 파일이 `~/.cache/qmd/mcp-<name>.pid|log`로 분리되고 설정 파일도 `<name>.yml`이 된다. 여기에 `QMD_CONFIG_DIR`과 `INDEX_PATH`로 설정과 DB를 `.orai/shelf/` 안에 둔다. 모델 캐시는 공유하므로 프로젝트마다 모델을 다시 받지 않는다. 서버는 `127.0.0.1`에만 열고, 포트는 설정값이 없으면 ID에서 유도한다(18200–18999). 이미 열린 포트는 MCP `status`의 컬렉션 절대경로가 이 프로젝트와 같을 때만 재사용한다. 중지는 index 범위의 `qmd --index <ours> mcp stop`만 쓴다. 역할 실행 시에는 MCP 서버 `shelf`를 프로세스 인자로 주입하므로 전역·worktree 설정 파일을 바꾸지 않는다. 역할 세션 밖에서 쓰려고 사용자가 프로젝트 MCP 설정(`.mcp.json`, `.codex/config.toml`)에 주소를 적어 둔 경우, Orai는 그 파일을 읽기만 하고 주소가 이 프로젝트 서버와 맞는지 진단한다(`shelf.registration`).

**CodeGraph.** 설치, provider MCP 등록, 프로젝트 색인은 사용자가 따로 진행하는 단계다. Orai는 `codegraph status --json`과 (deep 모드에서) 알려진 심볼 조회로 상태만 진단한다. 전역 등록과 중복될 수 있어 MCP를 자동으로 주입하지 않는다.

## 진단 모델

`doctor`는 읽기 전용이다. 구성요소마다 `component`, `status`, `reason`, `next_action`, `checked_at`을 낸다. 기본 출력은 사람이 읽는 표와 순서 있는 조치 목록이고, `--json`은 같은 내용에 `detail`을 더한 JSON이다. 두 출력의 종료 코드는 같다. `next_action`은 실행할 명령이나 전체 URL로 적는다. Orai를 쓰는 프로젝트에는 이 저장소의 문서가 없으므로 `docs/...` 같은 상대 경로로 안내하지 않는다.

| status | 의미 |
|---|---|
| healthy | 확인한 범위에서 정상 |
| degraded | 동작하지만 일부 기능이나 신뢰도가 떨어짐 |
| not-ready | 설정은 됐지만 아직 준비 전 (색인 없음·비어 있음, 의미 검색 미실행 등) |
| blocked | 이 구성요소를 사용할 수 없음 |
| not-configured | 설정하지 않음 |
| not-checked | 이 모드에서 일부러 실행하지 않음 (예: `--deep` 없이 의미 검색). 전체 상태에 영향 없음 |
| unsupported | 설치된 버전에 필요한 기능이 없음 |

shelf 진단은 원인별로 나눈다. 연결 거부, 샌드박스 접근 거부, 다른 프로젝트 서버, MCP handshake 실패, 빈 색인, embedding 미완료, vector 검색 실패, lex+vec·본문 조회 실패가 각각 다른 항목으로 보고된다. 색인이 선언과 다른 경우(`orai.toml`에서 지운 묶음이 남아 있음, 두 묶음이 같은 문서를 덮음)는 `shelf.collections`로, 선언한 폴더 설명이 색인과 다른 경우는 `shelf.context`로(설명을 선언한 프로젝트에만 나오고, 엔진에 물어볼 수 없으면 not-checked다), 프로젝트 MCP 설정의 주소 불일치는 `shelf.registration`으로 보고한다. 기본 doctor는 모델을 불러오지 않으므로 의미 검색 항목을 not-checked로 표시한다. not-checked는 검증했다는 뜻이 아니지만, 문제로 세지도 않는다. 의미 검색까지 확인하려면 `--deep`을 쓴다. 한 환경(예: 샌드박스 밖)에서 성공했다고 다른 환경에서도 성공했다고 확대하지 않는다.

종료 코드: 0 healthy / 1 degraded(또는 코어가 not-configured) / 2 사용법·설정 오류 / 3 코어 blocked. 메시지 명령은 성공 0, 실패 1, 사용법 오류 2다.

## 모듈 지도

| 패키지 | 책임 |
|---|---|
| `cmd/orai` | 진입점 (빌드한 바이너리의 종단 테스트 포함) |
| `internal/cli` | 인자 파싱, `orai <role>` 별칭, 출력, 종료 코드 |
| `internal/project` | 프로젝트 루트·worktree 해석, ID, 이동·복사 감지, setup 대상 |
| `internal/config` | `orai.toml` schema 검증 |
| `internal/state` | 원자적 저장, 역할 lock, 이전 상태 이력 |
| `internal/mail` | AMQ 호환 메일함: 전송·답장·목록·수신·읽기, 변경 감시 |
| `internal/runtime` | 실행·캡처·재개 검증, status, 프로젝트 진단 |
| `internal/providers` | Codex·Claude 인자, Codex queue notifier |
| `internal/channel` | Claude MCP 채널 (`orai _channel`) |
| `internal/shelf` | shelf 엔진(QMD) 수명주기·진단, 역할별 MCP 주입 |
| `internal/codegraph` | CodeGraph 진단과 setup 단계 |
| `internal/doctor` | 진단 상태 모델, 사람용·JSON 출력, 설치 안내 문구, 도구 capability·로그인 검사 |
| `internal/scaffold` | `orai setup`의 파일 계획·적용, 내장 템플릿(`go:embed`) |
| `internal/setup` | `orai setup` 전체 흐름: 파일 → shelf → 코드 그래프 → 진단 |
| `internal/version` | 버전 (릴리스 빌드 시 `-ldflags`로 주입) |
