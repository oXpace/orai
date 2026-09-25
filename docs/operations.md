# 운영 안내

이 문서는 설치, 초기화, 시작·중지, 진단, 복구 절차를 소유한다. 설계 근거는 [아키텍처](architecture.md), 버전 요구는 [호환성](compatibility.md)을 따른다. 에이전트의 메시지 사용법은 `orai setup`이 배포하는 `.agents/skills/orai/SKILL.md`에 있다.

## 설치

Orai는 **프로젝트 단위**로 설치한다. 프로젝트마다 `mise.toml`에 버전을 고정하므로, 프로젝트별로 다른 Orai 버전을 쓸 수 있고 같은 저장소를 받은 사람은 같은 버전을 쓴다.

```sh
mkdir my-app && cd my-app
mise use github:oXpace/orai@<버전>  # GitHub Release의 바이너리를 받아 mise.toml에 고정
orai setup                         # 기본 세팅
```

- mise github backend는 `oXpace/orai`의 GitHub Release에서 현재 플랫폼(darwin/linux, arm64/amd64)의 `orai_<버전>_<os>_<arch>.tar.gz`를 받는다. 실행에 Go나 다른 런타임은 필요 없다. 게시 전 변경은 Orai checkout에서 `go run ./cmd/orai --project <경로> setup`으로 시험한다.
- 0.1.0(Python)은 `pypi:oXpace/orai@0.1.0`으로 고정돼 있다. 0.2.0부터는 `github:` 백엔드를 쓴다.
- `setup`은 프로젝트에 Orai 고정이 없으면 `mise use` 안내를 출력한다.
- mise에 `minimum_release_age` 설정이 있으면 새 Release가 버전 목록(`mise ls-remote`)에서 한동안 숨겨진다. 이때도 `@0.2.0`처럼 버전을 명시하면 설치된다.
- PATH에 다른 `orai`(예: 예전 전역 링크)가 있어도, mise가 활성화된 셸에서는 프로젝트에 고정한 버전이 먼저 선택된다.

Codex, Claude Code, QMD, CodeGraph는 사용자가 설치한다. Orai는 이 도구들을 설치하거나 업그레이드하지 않는다. AMQ는 필요 없다(메일함은 Orai가 관리하며 형식만 AMQ와 호환된다). 필요한 버전은 [호환성](compatibility.md)에 있다.

Orai 자체를 개발하는 checkout에서는 다음과 같이 준비한다.

```sh
mise install && mise run setup    # Go pin, go mod download
mise run build                    # dist/orai
```

## 프로젝트 준비 (`orai setup`)

```sh
orai setup                        # 적용 (여러 번 실행해도 안전)
orai setup --dry-run              # 바꿀 내용만 출력
orai setup --preset pm-staff      # PM·STAFF 예제 역할 포함
orai setup --no-tools             # 파일만 준비 (wiki 서버·코드 그래프 생략, CI 등)
orai setup --branch <이름>        # 새 저장소의 기본 브랜치 (기본 trunk)
```

대상 폴더는 `--project`로 지정할 수 있다. 지정하지 않으면 현재 저장소의 기존 프로젝트, 없으면 저장소의 main checkout, Git 밖이면 현재 폴더가 대상이다. 저장소 바깥 상위 폴더의 `orai.toml`은 다른 프로젝트로 보고 사용하지 않는다. 모노레포 하위 프로젝트를 처음 만들 때는 `--project`로 지정한다.

`setup`은 먼저 모든 파일 변경의 충돌을 모아 보고한다. 충돌이 하나라도 있으면 아무 파일도 바꾸지 않는다. 적용 순서는 다음과 같다.

1. **Git 저장소**: 저장소 밖이면 `git init -b trunk`로 만든다. 이미 저장소 안이면(모노레포 포함) 건드리지 않는다.
2. **파일**:
   - `orai.toml`: 없을 때만 만든다. 있으면 검증만 한다.
   - `AGENTS.md`, `.gitignore`: `orai:begin`/`orai:end` 관리 블록만 추가하거나 교체한다. 블록 밖 내용은 바꾸지 않는다.
   - `.agents/skills/orai/SKILL.md`: 생성 표식이 있는 파일만 갱신한다. 표식 없이 사용자가 만든 파일이 있으면 충돌로 보고한다.
   - `.claude/skills/orai` 심볼릭 링크를 만든다.
   - `CLAUDE.md`: 없을 때만 `@AGENTS.md`로 만든다.
   - 역할 지침(프리셋): 없을 때만 만든다.
   - wiki 폴더가 없으면 `docs/README.md` 시작 페이지를 만든다.
   - `.orai/project.json`을 기록한다.
   - 기존 파일을 바꿀 때는 원본을 `.orai/backups/<시각>/`에 먼저 저장한다.
3. **메일함** (역할이 있는 프로젝트): `.agent-mail/<session>`에 역할과 `user`의 메일함을 만든다. 기존 메일함이 있으면 없는 handle만 추가하고 메시지는 건드리지 않는다. `.gitignore` 블록에 `/.agent-mail/`와 루트 안의 역할 worktree가 추가된다.
4. **wiki**: 엔진(QMD)이 설치돼 있고 문서가 있으면 색인이 없을 때 `orai wiki init`을, 있으면 `orai wiki recover`를 실행한다. 두 경우 모두 서버를 시작하고 검증한다.
5. **코드 그래프**: CodeGraph가 설치돼 있고 색인이 없으면 `codegraph init`을 실행한다.
6. **진단**: `orai doctor`를 요약해 남은 조치만 보여준다.

도구가 없으면 해당 단계는 건너뛰고 이유를 출력한다. 도구 단계가 실패하면 종료 코드 1로 끝난다. `pm-staff` 프리셋의 staff worktree는 첫 커밋 뒤에 `git worktree add .worktrees/staff`로 만든다.

## 역할 실행

```sh
orai run <role>            # 또는 orai <role>. 기본은 캡처된 대화의 정확한 재개
orai run <role> --fresh    # 새 대화 (이전 대화는 provider에 남고 상태는 .orai/history로)
orai run <role> --dry-run  # 실행할 명령·경로·큐만 출력, 아무것도 만들지 않음
orai status                # 실행 여부, 세션 ID, 알림 준비, 미처리 메시지 수
```

역할마다 터미널 하나에서 실행한다. 실행 중인 역할을 두 번 시작하면 거부된다. 최초 실행 때 Codex는 `/hooks`에서 `_capture` hook 신뢰를, Claude는 `server:orai` development channel 동의를 요구할 수 있다. 이 동의가 일반 도구 권한을 우회하지는 않는다. 조직 정책으로 channels가 막혀 있으면 Claude 알림은 동작하지 않는다.

중지는 해당 CLI를 정상 종료한다. SIGTERM·SIGHUP은 자식 CLI에 전달되고, 종료 후 상태는 `stopped`가 된다. PID만 보고 lock 파일을 지우지 않는다.

**역할 추가.** `orai.toml`에 역할을 추가하고 `orai setup` 또는 `orai run <새 역할>`을 실행하면 새 handle의 메일함이 추가된다. 기존 메시지는 건드리지 않으며, 이미 등록된 handle(은퇴한 역할 포함)은 `meta/config.json`에서 빠지지 않는다.

## 메시지

```sh
orai msg inbox [<ID>] [--peek] [--limit N]
orai msg send <role|user> [--kind K] [--thread T] (--body 텍스트 | --file 경로 | 표준 입력)
orai msg reply <받은-ID> (--body | --file | 표준 입력)
```

역할 세션 밖(Desktop)에서는 사용자 권한으로 `--as user`를 붙인다. 역할 세션 안에서는 `--as`를 쓸 수 없다. 본문 없이 대화형 터미널에서 실행하면 기다리지 않고 바로 오류를 낸다. 결과는 JSON으로 출력한다(형태는 AMQ의 `send`/`list`/`drain`/`read`/`reply` 출력과 같다). 성공 0, 실패 1, 사용법 오류 2로 끝난다.

## 진단

```sh
orai doctor           # 읽기 전용. 모델을 불러오지 않음
orai doctor --deep    # QMD vector·lex+vec·본문 조회, CodeGraph smoke 심볼까지
```

| 종료 코드 | 의미 |
|---|---|
| 0 | 모든 확인 항목 healthy (선택 연동의 not-configured 포함) |
| 1 | 일부 degraded / not-ready / 선택 연동 blocked, 또는 프로젝트를 찾지 못함 |
| 2 | 사용법·설정 오류 |
| 3 | 코어(사용 중인 provider와 로그인, 메일함) blocked |

각 항목의 `next_action`은 안전한 다음 조치다. `doctor`는 아무 상태도 바꾸지 않는다. 복구는 아래의 명시적 명령으로 한다.

## 프로젝트 wiki (문서 검색)

wiki는 `integrations.wiki.collections`의 Markdown 폴더(기본 `docs/`)를 색인한 프로젝트 문서 검색이다. 엔진은 QMD이며(현재 유일), 사용자는 `orai wiki`만 쓴다. 문서를 고친 뒤에는 `orai wiki stop && orai wiki refresh`로 색인을 갱신한다.

```sh
orai wiki init      # 설정이 없을 때만 생성(기본 모델 Qwen3-Embedding-0.6B Q8), update·embed, 서버 시작, 검증
orai wiki recover   # 기존 설정·DB·모델 보존, 꺼진 서버 시작, 검증 (재부팅 후 필요)
orai wiki refresh   # 문서·임베딩 갱신 후 시작·검증 (서버가 떠 있으면 거부)
orai wiki check     # 상태 변경 없이 전체 검증 (vector-only → lex+vec → 본문)
orai wiki stop      # 이 프로젝트 index의 서버만 중지
```

- 서버는 `127.0.0.1:<프로젝트 포트>`에서 실행하고, 포트는 `orai doctor`의 `wiki.server.detail.endpoint`로 확인한다. PID와 로그는 `~/.cache/qmd/mcp-orai-<id>.pid|log`에 있다. 다른 프로젝트 서버와 기존 Pockets `8181` 서버는 건드리지 않는다.
- 포트가 다른 index의 서버에 점유돼 있으면 거부하고 그대로 둔다. `orai.toml`의 `integrations.wiki.port`로 다른 포트를 지정한다.
- `init`·`refresh`는 검색이 없는 안전한 시점에 `orai wiki stop`을 먼저 실행한 뒤 수행한다.
- 손상되었거나 다른 프로젝트의 DB를 자동으로 지우지 않는다. 검색 도중 자동 재색인, 모델 다운로드, 서버 재시작도 하지 않는다.
- **모델 변경은 명시적 재구축 작업이다.** 서버를 중지하고, `.orai/wiki/orai-<id>.yml`의 `models.embed`를 바꾼 뒤 `QMD_CONFIG_DIR=.orai/wiki INDEX_PATH=.orai/wiki/index.sqlite qmd --index orai-<id> embed -f`를 실행하고 `orai wiki recover`로 검증한다. 모델 이름, revision, 파일 fingerprint, 메모리 요구량을 기록한다.
- 기본 모델은 한국어·영어 혼합 문서용 초기 후보일 뿐이며 한국어 품질을 보장하지 않는다. `integrations.wiki.smoke`에 이 프로젝트 문서에만 있는 사실을 넣어 recall을 점검한다.
- 현재 한계: 재부팅하면 서버가 내려간다. 첫 버전은 수동 `recover`만 제공하고, 상시 supervisor(launchd)는 필요와 지원 범위를 확인한 뒤 추가한다. CPU fallback(`QMD_FORCE_CPU=1`)은 설치된 바이너리에서 아직 검증하지 않았다.
- 역할 세션에는 MCP 서버 `wiki-<프로젝트 slug>`가 프로세스 인자로 자동 등록된다. 역할 밖의 CLI에서 쓰려면 같은 URL을 로컬 범위로 직접 등록한다(예: `claude mcp add --transport http --scope local wiki-orai <endpoint>`).

## CodeGraph

설치, MCP 등록, 색인은 서로 다른 단계이며 사용자가 명시적으로 수행한다.

```sh
codegraph install --print-config claude      # 파일을 쓰지 않고 등록 설정만 확인
codegraph install --target claude --location local   # 프로젝트 로컬 등록 (전역 변경 회피)
codegraph init <프로젝트 루트>                  # 초기 코드가 생긴 뒤 색인
codegraph status --json <프로젝트 루트>
```

`orai doctor`는 색인 상태와 미동기화 변경을, `--deep`은 `integrations.codegraph.smoke_symbol` 조회를 확인한다. 자동 sync가 있더라도 실제 조회 결과로 확인한다. 빈 검색이나 부정확한 호출 관계를 코드가 없다는 근거로 삼지 않는다.

## 복구 표

| 증상 | 조치 |
|---|---|
| `already running` | 기존 터미널을 사용한다. lock을 강제로 지우지 않는다 |
| 저장된 대화 없음·경로 변경 | transcript나 worktree를 되살리거나 `--fresh`. 최근 대화를 추측하지 않는다 |
| 세션 ID 미수집 | Codex `/hooks` 신뢰, Claude SessionStart hook 오류를 확인한 뒤 `--fresh` |
| Claude 알림 준비 안 됨 | channel 동의·MCP 연결·`channel_ready` 호출 여부를 `orai status`로 확인. 초기 연결 중 잠깐 `no MCP server configured`가 보일 수 있다 |
| Codex queue 오류 | `orai status`의 `last_error`를 확인한다. 실패하는 동안에도 메시지는 메일함에 남는다 |
| 프로젝트 이동·복사 경고 | `.orai/`를 검토한 뒤 `orai setup`으로 다시 연결하고, 역할은 `--fresh`로 시작 |
| QMD `connection refused` | `orai wiki recover` |
| QMD `access denied` | 샌드박스 밖에서 다시 확인한다. 그 결과는 그 환경에만 적용된다 |
| QMD 다른 index 서버 | 그대로 두고 `integrations.wiki.port`를 바꾼다 |

## 검증

```sh
mise run check           # gofmt + go vet + test (CI와 같음)
mise run test            # race 검사기, 빌드한 바이너리를 checkout 밖에서 실행하는 종단 테스트 포함
                         # (AMQ가 설치돼 있으면 메일함 양방향 호환 테스트도 실행)
```

테스트는 실제 역할 세션, 실제 프로젝트 메일함, `8181` 서버를 건드리지 않는다. fake provider와 fixture로 통과한 것을 실제 provider 동작으로 기록하지 않는다.

### 실제 파일럿 (계정·호스트 준비 후)

격리된 작은 파일럿 저장소에서 수행한다. Orai 저장소와 Pockets에서는 하지 않는다.

1. 빈 폴더에서 `mise use github:oXpace/orai@<버전>`, `orai setup --preset pm-staff`, 첫 커밋, `git worktree add .worktrees/staff`, `orai doctor`.
2. 두 터미널에서 `orai pm`, `orai staff`를 실행한다. Codex `/hooks` 신뢰와 Claude channel에 동의한다.
3. 요청과 회신을 주고받는다(`send` → 알림 → `inbox <ID>` → `reply`). 작업 중 알림, 유휴 알림, 수신자가 꺼져 있을 때의 적재, 재접속 후 알림을 각각 확인한다.
4. 역할을 종료한 뒤 다시 실행해 같은 UUID로 재개되고 메시지를 받는지 확인한다. 중복 실행이 거부되는지 확인한다.
   - 설치 버전에서 아직 확인하지 않은 전제가 있다. Claude `--resume <UUID>`의 SessionStart hook이 같은 `session_id`를 보내야 한다(다르면 재개할 때마다 캡처가 거부된다). Codex SessionStart payload에는 `cwd`와 `transcript_path`가 있어야 하고, `codex resume` 때도 hook이 실행돼야 한다(실행되지 않으면 Codex 알림이 준비 상태가 되지 않는다). 시작과 재개 때 한 번씩 hook 입력을 기록해 확인한다.
5. `orai wiki init` 후 `orai doctor --deep`으로 실제 검색을, `codegraph init` 후 smoke 심볼 조회를 확인한다.

결과는 [이력](history.md)의 검증 기록에 날짜, 버전, 통과·미검증 범위와 함께 남긴다.
