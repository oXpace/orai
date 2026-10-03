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
orai setup                        # 역할 없이 준비 (여러 번 실행해도 안전)
orai setup --role lead=codex --role dev=claude   # 역할을 정해 준비
orai setup --role reviewer=claude # 기존 프로젝트에 역할 추가
orai setup --preset pm-staff      # --role pm=codex --role staff=claude 의 줄임
orai setup --dry-run              # 바꿀 내용만 출력
orai setup --no-tools             # 파일만 준비 (wiki 서버·코드 그래프 생략, CI 등)
orai setup --branch <이름>        # 새 저장소의 기본 브랜치 (기본 trunk)
orai setup --help                 # 옵션과 예시
```

**역할 정하기.** `--role 이름=provider[:작업폴더]`를 역할마다 붙인다. provider는 `codex` 또는 `claude`다.

- 이름은 소문자로 시작하는 영숫자·하이픈이며 명령 이름(`setup`, `run`, `user` 등)은 쓸 수 없다.
- 작업 폴더를 생략하면 프로젝트의 첫 역할은 프로젝트 폴더(`.`)를, 그다음 역할부터는 `.worktrees/<이름>`을 쓴다. 직접 정하려면 `--role dev=claude:../app-dev`처럼 적는다. `:.`은 프로젝트 폴더를 함께 쓴다.
- 새 역할마다 `orai.toml`에 `[roles.<이름>]`, 지침 파일 `.agents/roles/<이름>.md`, 메일함이 생긴다. 지침 파일은 책임·경계를 적는 틀이며 이후 `setup`이 덮어쓰지 않는다.
- 이미 `orai.toml`이 있으면 새 역할의 표만 파일 끝에 덧붙인다. 원본은 `.orai/backups/`에 남는다. 이미 선언된 역할은 그대로 두고, provider가 다르면 충돌로 보고한 뒤 아무것도 바꾸지 않는다.
- 세션을 열 때의 공통 단계(문서 읽기, 메시지 확인, 할 일이 없으면 턴 종료)는 프롬프트에 있다. 일부 역할에만 필요한 단계는 역할 지침의 "세션 시작" 절에 둔다. `setup`은 Claude 역할의 지침에 이 절(`channel_ready` 호출)을 써 넣고, Codex 역할에는 넣지 않는다. 역할마다 세션을 열 때 할 일을 더 정하려면 이 절에 적는다. Codex 역할을 Claude로 바꿀 때는 따로 할 일이 없다(절에 `channel_ready`가 없으면 프롬프트가 직접 알려 준다). Claude 역할을 Codex로 바꿀 때는 이 절을 지운다.
- 역할 지침(`.agents/roles/`)은 커밋되지 않는다. 컴퓨터마다 `setup`이 틀을 만들고 각자 고쳐 쓴다. 팀이 같은 지침을 공유하려면 `orai.toml`의 `guide`를 `docs/roles/dev.md`처럼 저장소에 넣는 경로로 바꾼다. 예전 버전에서 이미 커밋한 지침은 `git rm -r --cached .agents/roles`로 추적만 해제한다(파일은 남는다).
- 역할 이름을 바꾸거나 역할을 없앨 때는 `orai.toml`의 `[roles.<이름>]` 표를 직접 고치거나 지운 뒤 `orai setup`을 실행한다. 새 이름의 메일함과, 없는 지침 파일(`guide`에 적은 경로)이 만들어진다. 예전 이름의 메일함과 지침 파일, 저장된 대화는 지우지 않으므로 필요하면 직접 정리한다. 이름을 바꾼 역할은 새 대화로 시작한다. `pm-staff`로 만든 프로젝트도 이 방법으로 원하는 구성으로 바꾼다.
- 별도 작업 폴더를 쓰는 역할은 첫 커밋 뒤에 `git worktree add .worktrees/<이름>`으로 폴더를 만든다. `setup`과 `doctor`가 이 명령을 남은 조치로 보여준다.

대상 폴더는 `--project`로 지정할 수 있다. 지정하지 않으면 현재 저장소의 기존 프로젝트, 없으면 저장소의 main checkout, Git 밖이면 현재 폴더가 대상이다. 저장소 바깥 상위 폴더의 `orai.toml`은 다른 프로젝트로 보고 사용하지 않는다. 모노레포 하위 프로젝트를 처음 만들 때는 `--project`로 지정한다.

`setup`은 먼저 모든 파일 변경의 충돌을 모아 보고한다. 충돌이 하나라도 있으면 아무 파일도 바꾸지 않는다. 적용 순서는 다음과 같다.

1. **Git 저장소**: 저장소 밖이면 `git init -b trunk`로 만든다. 이미 저장소 안이면(모노레포 포함) 건드리지 않는다.
2. **파일**:
   - `orai.toml`: 없으면 만든다. 있으면 검증하고, `--role`로 요청한 새 역할만 덧붙인다.
   - `AGENTS.md`, `.gitignore`, `.agents/.gitignore`: `orai:begin`/`orai:end` 관리 블록만 추가하거나 교체한다. 블록 밖 내용은 바꾸지 않는다. `AGENTS.md` 블록은 에이전트에게 Orai가 무엇이고 역할·메시지·진단·문서 검색을 어떻게 쓰는지 설명한다. `.agents/.gitignore` 블록은 `/roles/`를 제외해 역할 지침을 저장소에 넣지 않는다.
   - `.agents/skills/orai/SKILL.md`: 생성 표식이 있는 파일만 갱신한다. 표식 없이 사용자가 만든 파일이 있으면 충돌로 보고한다.
   - `.claude/skills/orai` 심볼릭 링크를 만든다.
   - `CLAUDE.md`: 없을 때만 `@AGENTS.md`로 만든다.
   - 역할 지침(`guide`에 적은 파일, 기본 `.agents/roles/<이름>.md`): 선언된 역할의 지침 파일이 없을 때만 만든다.
   - wiki 폴더가 없으면 `docs/README.md` 시작 페이지를 만든다.
   - `.orai/project.json`을 기록한다.
   - 기존 파일을 바꿀 때는 원본을 `.orai/backups/<시각>/`에 먼저 저장한다.
3. **메일함** (역할이 있는 프로젝트): `.agent-mail/<session>`에 역할과 `user`의 메일함을 만든다. 기존 메일함이 있으면 없는 handle만 추가하고 메시지는 건드리지 않는다. `.gitignore` 블록에 `/.agent-mail/`와 루트 안의 역할 worktree가 추가된다.
4. **wiki**: 엔진(QMD)이 설치돼 있고 문서가 있으면 색인이 없을 때 `orai wiki init`을, 있으면 `orai wiki recover`를 실행한다. 두 경우 모두 서버를 시작하고 검증한다.
5. **코드 그래프**: CodeGraph가 설치돼 있고 색인이 없으면 `codegraph init`을 실행한다.
6. **진단**: `orai doctor`와 같은 형식으로 문제 항목과 남은 조치(Next steps)만 보여준다.

도구가 없으면 해당 단계는 건너뛰고 설치 명령을 출력한다. 도구 단계가 실패하면 종료 코드 1로 끝난다.

## 역할 실행

```sh
orai run <role>            # 또는 orai <role>. 기본은 캡처된 대화의 정확한 재개
orai run <role> --fresh    # 새 대화 (이전 대화는 provider에 남고 상태는 .orai/history로)
orai run <role> --dry-run  # 실행할 명령·경로·큐만 출력, 아무것도 만들지 않음
orai status                # 실행 여부, 세션 ID, 알림 준비, 미처리 메시지 수
```

역할마다 터미널 하나에서 실행한다. 실행 중인 역할을 두 번 시작하면 거부된다. 최초 실행 때 Codex는 `/hooks`에서 `_capture` hook 신뢰를, Claude는 `server:orai` development channel 동의를 요구할 수 있다. 이 동의가 일반 도구 권한을 우회하지는 않는다. 조직 정책으로 channels가 막혀 있으면 Claude 알림은 동작하지 않는다.

중지는 해당 CLI를 정상 종료한다. SIGTERM·SIGHUP은 자식 CLI에 전달되고, 종료 후 상태는 `stopped`가 된다. PID만 보고 lock 파일을 지우지 않는다.

**역할 추가.** `orai setup --role <이름>=<provider>`를 실행한다. `orai.toml`을 직접 고친 경우에는 `orai setup` 또는 `orai run <새 역할>`을 실행하면 새 handle의 메일함이 추가된다. 기존 메시지는 건드리지 않으며, 이미 등록된 handle(은퇴한 역할 포함)은 `meta/config.json`에서 빠지지 않는다.

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
orai doctor --deep    # wiki의 실제 검색(vector, lex+vec, 본문 조회)과 CodeGraph 심볼 조회까지
orai doctor --json    # 같은 내용을 JSON으로 (detail 포함, 스크립트·에이전트용)
orai doctor --help    # 표시와 종료 코드 설명
```

기본 출력은 사람이 읽는 형식이다. 구성요소마다 한 줄씩 상태와 이유를 보여주고, 끝에 남은 조치를 실행할 명령으로 순서대로 적는다.

```text
  ✓ project         orai.toml is valid
  ✗ tool.claude     blocked: claude is not on PATH
  ! role.dev        degraded: missing role worktree: .worktrees/dev
  · wiki.search     not-checked: search itself was not run (it loads the search models)

Status: blocked (2 to fix)

Next steps:
  1. tool.claude: Install Claude Code: `curl -fsSL https://claude.ai/install.sh | bash` ...
  2. role.dev: `git worktree add .worktrees/dev` (from the project folder)

Optional:
  - wiki.search: `orai doctor --deep` runs real searches
```

| 표시 | 상태 | 뜻 |
|---|---|---|
| `✓` | healthy | 확인했고 정상 |
| `✗` | blocked, unsupported | 이 기능을 쓸 수 없음. 도구 없음, 로그인 안 됨, 필요한 옵션이 없는 버전 |
| `!` | degraded, not-ready | 일부만 동작하거나 아직 준비 전 |
| `-` | not-configured | `orai.toml`에서 켜지 않은 선택 기능. 문제가 아님 |
| `·` | not-checked | 이번 실행에서 확인하지 않음(`--deep` 필요). 문제도 검증도 아님 |

"Next steps"는 고쳐야 하는 항목이고 "Optional"은 해도 되고 안 해도 되는 항목이다. wiki와 코드 그래프는 선택 기능이므로, 쓰지 않을 프로젝트는 `orai.toml`에서 해당 `[integrations.*]` 표를 지우면 진단에서 빠진다.

**`--deep`이 확인하는 내용 정하기.** 설정이 없으면 `--deep`은 검색 결과가 나오는지만 확인한다. 올바른 문서가 나오는지까지 확인하려면 `orai.toml`에 이 프로젝트 문서에만 있는 사실을 적는다.

```toml
[integrations.wiki.smoke]
lex = "nonce"                                   # 그 문서에만 나오는 낱말
vec = "세션 ID를 어떻게 안전하게 수집하는가"        # 그 문서가 답하는 질문
expect = "docs/architecture.md"                 # 두 검색이 모두 돌려줘야 하는 문서 (프로젝트 폴더 기준 경로)

[integrations.codegraph]
smoke_symbol = "ValidateSaved"                  # 색인에 반드시 있어야 하는 심볼 이름
```

| 종료 코드 | 의미 |
|---|---|
| 0 | 모든 확인 항목 healthy (선택 연동의 not-configured 포함) |
| 1 | 일부 degraded / not-ready / 선택 연동 blocked, 또는 프로젝트를 찾지 못함 |
| 2 | 사용법·설정 오류 |
| 3 | 코어(사용 중인 provider와 로그인, 메일함) blocked |

JSON의 `next_action`이 "Next steps"에 나오는 조치다. `doctor`는 아무 상태도 바꾸지 않는다. 복구는 아래의 명시적 명령으로 한다.

## 프로젝트 wiki (문서 검색)

wiki는 `integrations.wiki.collections`의 Markdown 폴더(기본 `docs/`)를 색인한 프로젝트 문서 검색이다. 엔진은 QMD이며(현재 유일), 사용자는 `orai wiki`만 쓴다. 같은 안내가 `orai wiki --help`에 있다.

### 처음 준비

1. **QMD 설치**: `npm install -g @tobilu/qmd` (Node 22 이상). 프로젝트에 버전을 고정하려면 `mise use npm:@tobilu/qmd@<버전>`을 쓴다. Orai는 QMD를 설치하지 않는다.
2. **문서 넣기**: `docs/`(또는 `collections`에 적은 폴더)에 Markdown 파일을 둔다. `orai setup`은 폴더가 없으면 시작 페이지 `docs/README.md`를 만든다.
3. **색인 만들기**: `orai wiki init`. QMD가 설치돼 있고 문서가 있으면 `orai setup`이 이 단계를 대신 실행한다.
   - 첫 실행 때 임베딩 모델(약 0.6 GB)을, 첫 실제 검색(`orai doctor --deep` 포함) 때 검색용 모델(약 2 GB)을 `~/.cache/qmd/models`에 받는다. 모델은 프로젝트끼리 공유하므로 한 번만 받는다.
   - 색인과 설정은 이 프로젝트의 `.orai/wiki/`에만 생긴다.
4. **확인**: `orai doctor`에서 `wiki.*` 항목이 `✓`인지 본다. 실제 검색까지 확인하려면 `orai doctor --deep`을 쓴다.

### 설정 (`orai.toml`의 `[integrations.wiki]`)

| 키 | 기본값 | 설명 |
|---|---|---|
| `collections` | `{ docs = "docs" }` | 검색할 폴더. `이름 = "폴더"`(프로젝트 폴더 기준)를 여러 개 적을 수 있다. 예: `{ docs = "docs", notes = "notes" }` |
| `port` | 프로젝트 경로에서 계산 (18200~18999) | 계산된 포트를 다른 서버가 쓰고 있을 때만 지정한다 |
| `embed_model` | Qwen3-Embedding-0.6B Q8 | 첫 `orai wiki init` 때만 읽는다. 이후 변경은 아래 "모델 변경"을 따른다 |
| `[integrations.wiki.smoke]` | 없음 | `--deep`이 올바른 문서가 검색되는지 확인할 질의. [진단](#진단) 참고 |

`collections`에 폴더를 추가한 뒤 `orai wiki stop && orai wiki refresh`를 실행하면 새 폴더가 색인에 추가된다. 이미 색인된 이름의 폴더 경로를 바꾸는 것은 자동으로 하지 않는다. 기존 색인 설정을 덮어쓰지 않기 위해서이며, 이때는 어느 파일(`.orai/wiki/orai-<id>.yml`)을 고칠지 알려 주고 멈춘다. `[integrations.wiki]` 표가 없으면 wiki를 쓰지 않는 프로젝트로 본다.

### 평소 사용

| 상황 | 명령 |
|---|---|
| 문서를 추가하거나 고쳤다 | `orai wiki stop && orai wiki refresh` |
| 재부팅 뒤 서버가 꺼졌다 (`wiki.server`가 `✗`) | `orai wiki recover` |
| 지금 검색이 되는지 확인 | `orai wiki check` |
| 서버 끄기 | `orai wiki stop` |

### 명령

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
- **모델 변경**은 명시적 재구축 작업이다. 서버를 중지하고, `.orai/wiki/orai-<id>.yml`의 `models.embed`를 바꾼 뒤 `QMD_CONFIG_DIR=.orai/wiki INDEX_PATH=.orai/wiki/index.sqlite qmd --index orai-<id> embed -f`를 실행하고 `orai wiki recover`로 검증한다. 모델 이름, revision, 파일 fingerprint, 메모리 요구량을 기록한다.
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
| wiki 서버가 꺼져 있음(`wiki.server` blocked) | `orai wiki recover` |
| wiki가 아직 만들어지지 않음 | QMD 설치 후 `orai wiki init` ([처음 준비](#처음-준비)) |
| 문서를 고쳤는데 검색에 안 나옴(`wiki.index` degraded) | `orai wiki stop && orai wiki refresh` |
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
