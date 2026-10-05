# 운영 안내

이 문서는 설치, 초기화, 시작·중지, 진단, 복구 절차를 소유한다. 설계 근거는 [아키텍처](architecture.md), 버전 요구는 [호환성](compatibility.md)을 따른다. 에이전트의 메시지 사용법은 `orai setup`이 배포하는 `.agents/skills/orai/SKILL.md`에 있다.

## 설치

Orai는 **프로젝트 단위**로 설치한다. 프로젝트마다 `mise.toml`에 버전을 고정하므로, 프로젝트별로 다른 Orai 버전을 쓸 수 있고 같은 저장소를 받은 사람은 같은 버전을 쓴다.

```sh
mkdir my-app && cd my-app
mise use --pin github:oXpace/orai  # 최신 GitHub Release의 바이너리를 받아 그 버전을 mise.toml에 고정
orai setup                         # 기본 세팅
```

특정 버전을 쓰려면 `mise use github:oXpace/orai@0.5.0`처럼 번호를 붙인다. Release 태그에 `v`가 없으므로 `@v0.5.0`이 아니라 `@0.5.0`이다.

- mise github backend는 `oXpace/orai`의 GitHub Release에서 현재 플랫폼(darwin/linux, arm64/amd64)의 `orai_<버전>_<os>_<arch>.tar.gz`를 받는다. 실행에 Go나 다른 런타임은 필요 없다. 게시 전 변경은 Orai checkout에서 `go run ./cmd/orai --project <경로> setup`으로 시험한다.
- `setup`은 프로젝트에 Orai 고정이 없으면 `mise use` 안내를 출력한다.
- mise는 나온 지 얼마 안 된 Release를 버전 목록(`mise ls-remote github:oXpace/orai`)에서 숨긴다. 이때 `--pin`은 이전 버전을 고르므로 `@0.5.0`처럼 번호를 명시한다. 번호를 명시한 설치는 배포 직후에도 된다. mise 2026.10.0에서 `minimum_release_age`를 설정하지 않았는데도 숨겨졌고, `mise ls-remote`가 "newer releases hidden by minimum_release_age"라고 알린다. 숨기는 기간의 기본값은 확인하지 못했다. 제외 설정 `minimum_release_age_excludes`가 있다.
- PATH에 다른 `orai`(예: 예전 전역 링크)가 있어도, mise가 활성화된 셸에서는 프로젝트에 고정한 버전이 먼저 선택된다.

Codex, Claude Code, QMD, CodeGraph는 사용자가 설치한다. Orai는 이 도구들을 설치하거나 업그레이드하지 않는다. AMQ는 필요 없다(메일함은 Orai가 관리하며 형식만 AMQ와 호환된다). 필요한 버전은 [호환성](compatibility.md)에 있다.

Orai 자체를 개발하는 checkout에서는 다음과 같이 준비한다.

```sh
mise install && mise run setup    # Go pin, go mod download
mise run build                    # dist/orai
```

## 버전 올리기와 마이그레이션

모든 버전에 공통인 절차는 다음과 같다. 무엇이 바뀌었는지는 [릴리스 노트](https://github.com/oXpace/orai/releases)에 있고, 이 절은 올릴 때 **해야 할 일**만 적는다.

```sh
mise use --pin github:oXpace/orai   # 고정 버전을 최신 릴리스로 바꾼다 (또는 @<버전>)
orai setup --dry-run                # 바뀔 파일 확인
orai setup                          # 스킬, AGENTS.md 블록, .gitignore 블록을 새 버전에 맞게 갱신
orai doctor                         # 남은 조치 확인
git add -A && git commit -m "Update Orai"
```

- `setup`은 자기가 만든 부분(생성 표식이 있는 스킬, `orai:begin`~`orai:end` 블록)만 갱신하고, 바꾸기 전 원본을 `.orai/backups/`에 둔다. `orai.toml`, 역할 지침, 블록 밖 내용은 고치지 않는다.
- 저장된 대화, 메일함, shelf 색인은 그대로 남는다. 실행 중인 역할 세션은 예전 버전으로 계속 돌므로, 끝낸 뒤 `orai <역할>`로 다시 연다. 같은 대화로 이어진다.
- 같은 저장소를 쓰는 다른 사람은 변경을 받은 뒤 `mise install`과 `orai setup`을 실행한다.
- 되돌리려면 `mise.toml`의 버전을 이전 번호로 바꾸고 `orai setup`을 다시 실행한다.

### 0.4.x → 0.5.0

반드시 해야 할 일은 없다. `orai setup`이 `AGENTS.md`의 Orai 블록과 `.gitignore` 블록을 새 내용으로 바꾼다.

| 대상 | 해야 할 일 |
|---|---|
| 컴퓨터마다 다른 설정(역할의 모델, 포트, 나만 쓰는 역할) | 새 기능이다. `orai.toml` 옆에 `orai.local.toml`을 만들어 적는다([공유 설정과 로컬 설정](#공유-설정과-로컬-설정)). 만들지 않으면 지금과 똑같이 동작한다. `orai setup`이 `.gitignore` 블록에 이 파일을 더한다 |
| `orai.toml`에 개인 값이 섞여 있을 때 | 옮기는 일은 직접 한다. 예를 들어 `[roles.pm]`의 `model`·`effort` 줄을 `orai.local.toml`의 `[roles.pm]`으로 옮긴다. 공유 기본값으로 남겨 두고 로컬에서 덮어써도 된다. Orai가 값을 자동으로 옮기지 않으므로 동작이 저절로 바뀌지 않는다 |
| 역할을 쓰지 않는 컴퓨터의 `orai doctor` | 이 컴퓨터에서 한 번도 시작하지 않았고 작업 폴더도 없는 역할은 문제로 세지 않고 선택 항목(`-`)으로 나온다. 전에는 degraded였다 |
| `orai status`를 읽는 스크립트 | 역할마다 `worktree`, `model`, `effort`, `local`이 추가됐다. 있던 필드는 그대로다 |
| 폴더별 설명(context)을 쓰고 싶을 때 | 새 기능이다. `orai.toml`에 `[integrations.shelf.context]`를 적고 `orai shelf recover`를 실행한다([폴더에 설명 달기](#폴더에-설명-달기-context)) |
| `qmd context add`로 직접 넣어 둔 설명이 있을 때 | 표를 적지 않는 동안에는 그대로 남는다. 표를 적으면 색인의 설명은 표와 같아지므로, 남길 설명을 먼저 `orai.toml`에 옮겨 적는다. 표에 없는 설명은 다음 `recover`·`refresh`가 본문을 출력한 뒤 지운다 |
| `orai doctor --json`을 읽는 스크립트 | 설명을 선언한 프로젝트에 `shelf.context` 항목이 추가된다. 로컬 파일이 있으면 `project.detail.local`(`file`, `keys`)이 추가된다. 시작한 적 없는 역할의 `role.<이름>`은 `degraded`가 아니라 `not-configured`다 |

### 0.3.x → 0.4.0

문서 검색의 이름이 `wiki`에서 `shelf`로 바뀌었다. 색인과 서버, 포트는 그대로다.

| 대상 | 해야 할 일 |
|---|---|
| `orai.toml` | `schema = 1`을 `schema = 2`로, `[integrations.wiki]`(와 `[integrations.wiki.smoke]`)를 `[integrations.shelf]`로 고친다. 고치기 전에도 예전 표를 그대로 읽으므로 동작은 멈추지 않고, `orai doctor`가 이 수정을 Next step으로 알려 준다. Orai는 `orai.toml` 본문을 대신 고치지 않는다 |
| 명령 | `orai wiki …` → `orai shelf …`. 예전 이름을 치면 새 이름을 알려 주고 끝난다 |
| 색인 폴더 `.orai/wiki/` | 손댈 것이 없다. 올린 뒤 첫 `orai setup`(또는 `orai shelf recover`)이 서버를 멈추고 `.orai/shelf/`로 옮긴 뒤 다시 시작한다. 색인을 다시 만들지 않는다 |
| MCP 서버 이름 | `wiki-<프로젝트>` → `shelf`(모든 프로젝트에서 같다). 역할 세션은 다시 열면 새 이름으로 연결된다. 에이전트 도구 이름이 `mcp__wiki-<프로젝트>__*`에서 `mcp__shelf__*`로 바뀌므로, 저장해 둔 도구 허용 목록과 프로젝트 지침의 이름을 고친다 |
| 역할 밖에서 직접 등록한 MCP 설정(`.codex/config.toml`, `.mcp.json`) | 항목 이름을 `shelf`로 바꾼다. 주소는 같다. `orai doctor`의 `shelf.registration`이 예전 이름과 다른 포트를 찾아 알려 준다([역할 세션 밖에서 쓰기](#역할-세션-밖에서-쓰기)) |
| `orai doctor --json`을 읽는 스크립트 | 구성요소 이름 `wiki`, `wiki.*`가 `shelf`, `shelf.*`가 됐고 `shelf.collections`, `shelf.registration`이 추가됐다 |
| 폴더를 나눠 등록하고 싶을 때 | 새 기능이다. [설정](#설정-oraitoml의-integrationsshelf)의 `pattern`을 쓴다 |

### 0.2.x → 0.3.0

| 대상 | 해야 할 일 |
|---|---|
| `orai doctor` 출력을 JSON으로 읽는 스크립트 | `orai doctor --json`으로 바꾼다. 필드와 종료 코드는 같다 |
| 저장소에 커밋돼 있는 `.agents/roles/` | `git rm -r --cached .agents/roles` 뒤 커밋한다. 파일은 남고 추적만 해제된다. 팀이 같은 지침을 공유하려면 대신 `orai.toml`의 `guide`를 `docs/roles/dev.md`처럼 저장소에 넣는 경로로 옮긴다 |
| 이전 버전이 만든 역할 지침 | 그대로 써도 된다. "세션 시작" 절이 없으면 프롬프트가 시작 절차를 직접 알려 준다. 새 형식에 맞추려면 Claude 역할의 지침에 아래 절을 넣는다 |
| `pm-staff`로 만든 역할 구성 | 바꾸지 않아도 된다. 다른 구성으로 바꾸려면 `orai.toml`의 `[roles.*]`를 고치거나 지우고 `orai setup`을 실행한다([역할 정하기](#프로젝트-준비-orai-setup)) |
| 받은 메시지를 `orai msg inbox <ID>`로 다시 읽는 스크립트 | 이미 받은 메시지는 본문 없이 `already_received`만 나온다. 본문이 필요하면 `--again`을 붙인다 |
| `orai.toml` | 바꿀 것이 없다(`schema = 1` 그대로) |

Claude 역할의 지침에 넣는 절:

```markdown
## 세션 시작

세션을 새로 시작하거나 다시 열 때마다 한다.

1. orai MCP의 `channel_ready` 도구를 호출해 수신 준비를 알린다. 호출하기 전에는 새 메시지 알림이 오지 않는다.
```

### 0.1.0 → 0.2.0 이상

0.1.0은 Python 패키지였고 AMQ가 필요했다. 0.2.0부터는 실행 파일 하나이며 AMQ가 필요 없다.

| 대상 | 해야 할 일 |
|---|---|
| 설치 원천 | `mise.toml`의 `"pypi:oXpace/orai"` 줄을 지우고 `mise use --pin github:oXpace/orai`를 실행한다 |
| `.amqrc` | 더 쓰지 않는다. 지워도 된다 |
| AMQ | 지워도 된다. 계속 설치해 두면 `amq`로 같은 메일함을 읽고 보낼 수 있다 |
| 메일함(`.agent-mail/<session>`) | 위치와 형식이 같아 기존 메시지가 그대로 읽힌다. `orai setup`이 빠진 폴더만 채운다 |
| 명령과 `orai.toml` | 바꿀 것이 없다 |

Orai가 독립 프로젝트가 되기 전의 방식(Pockets의 `scripts/orai`와 `.agents/orai.json`)에서 옮기는 일은 [배포 계획](release.md#단계)의 "소비 프로젝트 이전" 단계가 다룬다.

## 프로젝트 준비 (`orai setup`)

```sh
orai setup                        # 역할 없이 준비 (여러 번 실행해도 안전)
orai setup --role lead=codex --role dev=claude   # 역할을 정해 준비
orai setup --role reviewer=claude # 기존 프로젝트에 역할 추가
orai setup --preset pm-staff      # --role pm=codex --role staff=claude 의 줄임
orai setup --dry-run              # 바꿀 내용만 출력
orai setup --no-tools             # 파일만 준비 (shelf 서버·코드 그래프 생략, CI 등)
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
- 별도 작업 폴더를 쓰는 역할은 첫 커밋 뒤에 `git worktree add .worktrees/<이름>`으로 폴더를 만든다. `setup`과 `doctor`가 이 명령을 보여준다. 이 컴퓨터에서 한 번도 시작하지 않은 역할은 선택 항목으로, 시작한 적이 있는데 폴더가 없어진 역할은 문제로 나온다.
- 이 컴퓨터에서만 쓸 역할은 `--local`을 붙인다. `orai.toml` 대신 `orai.local.toml`에 선언된다([공유 설정과 로컬 설정](#공유-설정과-로컬-설정)).

### 공유 설정과 로컬 설정

| 파일 | 담는 것 | 커밋 |
|---|---|---|
| `orai.toml` | 프로젝트가 공유하는 설정: 묶음, 폴더 설명, CodeGraph 사용 여부, 팀이 정한 역할과 그 기본값 | 한다 |
| `orai.local.toml` | 이 컴퓨터만의 설정. `orai.toml` 옆(main checkout)에 둔다. 없어도 된다 | 안 한다 |

로컬 파일은 공유 파일 위에 키 단위로 겹친다. 값은 공유 값을 대신하고, 로컬에만 있는 역할·묶음·설명은 더해진다.

```toml
# orai.toml (공유)
[roles.pm]
provider = "codex"
worktree = "."
guide = ".agents/management/PM.md"
model = "gpt-6-sol"            # 팀 기본값. 없으면 provider 기본값

[integrations.shelf]
collections = { docs = "docs" }
port = 18338
```

```toml
# orai.local.toml (이 컴퓨터)
[roles.pm]
model = "gpt-6.1-sol"          # pm의 모델만 바뀐다. provider, 작업 폴더, 지침은 공유 값 그대로
effort = "high"

[roles.scratch]                # 이 컴퓨터에만 있는 역할
provider = "claude"
worktree = ".worktrees/scratch"

[integrations.shelf]
port = 18401                   # 이 컴퓨터에서 포트가 겹칠 때
```

- **우선순위**: 로컬이 이긴다. 표(`[roles.pm]`, `[integrations.shelf.collections]` 등)는 같은 이름의 표와 합치고, 그 밖의 값은 통째로 대신한다. 설정에 목록 값은 없다.
- **경로**: 어느 파일에 적든 프로젝트 폴더 기준이다.
- **정할 수 없는 것**: `schema`, `name`, `session`. 프로젝트 ID, 색인 이름, 메일함 경로가 여기서 나온다.
- **지우기**: 로컬 파일로 공유 선언을 지우거나 끌 수는 없다. 더하거나 바꾸기만 한다. 쓰지 않는 역할은 그대로 둬도 `orai doctor`가 문제로 세지 않는다.
- **오류**: `orai.toml`은 혼자서도 유효해야 한다. 겹친 결과가 잘못되면 명령이 설정 오류(종료 코드 2)로 멈추고 `orai.local.toml`을 짚는다. 조용히 무시하지 않는다.
- **적용 값 확인**: `orai doctor`의 `project` 항목이 로컬 파일이 정한 키를 나열한다. `orai status`는 역할마다 적용 중인 `provider`, `worktree`, `model`, `effort`와, 그중 로컬 파일이 정한 것(`local`)을 보여 준다. `orai <역할> --dry-run`은 실제 실행 명령을 보여 준다.
- **setup**: `orai.local.toml`을 만들거나 고치지 않는다. `orai setup --role 이름=provider --local`일 때만 그 역할을 로컬 파일에 덧붙인다(없으면 만든다). `--local`이 없으면 지금처럼 `orai.toml`에 덧붙인다.
- **Git**: 커밋되는 `.gitignore` 블록은 공유 설정만으로 계산하므로 사람마다 달라지지 않는다. 로컬 설정 때문에 더 무시해야 하는 경로(로컬 역할의 작업 폴더 등)는 `setup`이 `.git/info/exclude`의 Orai 블록에 넣는다. 저장소 최상위가 아닌 프로젝트(모노레포 하위)에서는 넣지 않고 경로를 알려 준다.
- **바꾼 뒤**: `model`·`effort`는 다음 `orai <역할>`부터 적용되고 저장된 대화는 그대로 이어진다. `provider`나 `worktree`를 바꾸면 저장된 대화와 맞지 않아 재개를 거부하므로 `--fresh`로 시작한다(`orai.toml`을 고쳤을 때와 같다). `port`를 로컬에서 바꾸면 커밋된 `.mcp.json`·`.codex/config.toml`의 주소와 어긋난다. `orai doctor`의 `shelf.registration`이 알려 준다. shelf 설정을 바꾼 뒤의 명령은 [shelf](#shelf-문서-검색) 절과 같다.
- **새 checkout**: 로컬 파일 없이 공유 설정 전부가 복구된다. 로컬에만 둔 역할과 개인 값은 그 컴퓨터에만 있다.

대상 폴더는 `--project`로 지정할 수 있다. 지정하지 않으면 현재 저장소의 기존 프로젝트, 없으면 저장소의 main checkout, Git 밖이면 현재 폴더가 대상이다. 저장소 바깥 상위 폴더의 `orai.toml`은 다른 프로젝트로 보고 사용하지 않는다. 모노레포 하위 프로젝트를 처음 만들 때는 `--project`로 지정한다.

`setup`은 먼저 모든 파일 변경의 충돌을 모아 보고한다. 충돌이 하나라도 있으면 아무 파일도 바꾸지 않는다. 적용 순서는 다음과 같다.

1. **Git 저장소**: 저장소 밖이면 `git init -b trunk`로 만든다. 이미 저장소 안이면(모노레포 포함) 건드리지 않는다.
2. **파일**:
   - `orai.toml`: 없으면 만든다. 있으면 검증하고, `--role`로 요청한 새 역할만 덧붙인다. `orai.local.toml`은 읽기만 하고, `--local`로 요청한 역할만 덧붙인다.
   - `AGENTS.md`, `.gitignore`, `.agents/.gitignore`: `orai:begin`/`orai:end` 관리 블록만 추가하거나 교체한다. 블록 밖 내용은 바꾸지 않는다. `AGENTS.md` 블록은 역할이 없어도 누구나 쓰는 것(진단, 문서 검색, 코드 구조)을 먼저 적고, 역할 세션에서만 해당하는 것(역할, 메시지)을 뒤에 적는다. 원칙과 진입점만 담고, 설정과 명령의 상세는 도움말과 이 문서가 맡는다. `.agents/.gitignore` 블록은 `/roles/`를 제외해 역할 지침을 저장소에 넣지 않는다.
   - `.agents/skills/orai/SKILL.md`: 생성 표식이 있는 파일만 갱신한다. 표식 없이 사용자가 만든 파일이 있으면 충돌로 보고한다.
   - `.claude/skills/orai` 심볼릭 링크를 만든다.
   - `CLAUDE.md`: 없을 때만 `@AGENTS.md`로 만든다.
   - 역할 지침(`guide`에 적은 파일, 기본 `.agents/roles/<이름>.md`): 선언된 역할의 지침 파일이 없을 때만 만든다.
   - shelf에 등록한 폴더가 없으면 `docs/README.md` 시작 페이지를 만든다.
   - `.orai/project.json`을 기록한다.
   - 기존 파일을 바꿀 때는 원본을 `.orai/backups/<시각>/`에 먼저 저장한다.
3. **메일함** (역할이 있는 프로젝트): `.agent-mail/<session>`에 역할과 `user`의 메일함을 만든다. 기존 메일함이 있으면 없는 handle만 추가하고 메시지는 건드리지 않는다. `.gitignore` 블록에 `/.agent-mail/`와 루트 안의 역할 worktree가 추가된다.
4. **shelf**: 엔진(QMD)이 설치돼 있고 문서가 있으면 색인이 없을 때 `orai shelf init`을, 있으면 `orai shelf recover`를 실행한다. 두 경우 모두 서버를 시작하고 검증한다.
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
orai msg inbox [<ID>] [--again] [--peek] [--limit N]
orai msg send <role|user> [--kind K] [--thread T] (--body 텍스트 | --file 경로 | 표준 입력)
orai msg reply <받은-ID> (--body | --file | 표준 입력)
```

이미 받은 메시지를 ID로 다시 읽으면 본문 없이 머리말과 `"already_received": true`, 받은 시각만 나온다. 알림은 보낸 뒤 회수할 수 없어서, 에이전트가 전체 수신으로 먼저 받은 메시지의 알림이 뒤늦게 도착할 수 있다. 이때 같은 일을 두 번 하지 않게 하기 위해서다. 일부러 다시 읽으려면 `--again`을 붙인다.

역할 세션 밖(Desktop)에서는 사용자 권한으로 `--as user`를 붙인다. 역할 세션 안에서는 `--as`를 쓸 수 없다. 본문 없이 대화형 터미널에서 실행하면 기다리지 않고 바로 오류를 낸다. 결과는 JSON으로 출력한다(형태는 AMQ의 `send`/`list`/`drain`/`read`/`reply` 출력과 같다). 성공 0, 실패 1, 사용법 오류 2로 끝난다.

## 진단

```sh
orai doctor           # 읽기 전용. 모델을 불러오지 않음
orai doctor --deep    # shelf의 실제 검색(vector, lex+vec, 본문 조회)과 CodeGraph 심볼 조회까지
orai doctor --json    # 같은 내용을 JSON으로 (detail 포함, 스크립트·에이전트용)
orai doctor --help    # 표시와 종료 코드 설명
```

기본 출력은 사람이 읽는 형식이다. 구성요소마다 한 줄씩 상태와 이유를 보여주고, 끝에 남은 조치를 실행할 명령으로 순서대로 적는다.

```text
  ✓ project         orai.toml is valid
  ✗ tool.claude     blocked: claude is not on PATH
  ! role.dev        degraded: missing role worktree: .worktrees/dev
  · shelf.search    not-checked: search itself was not run (it loads the search models)

Status: blocked (2 to fix)

Next steps:
  1. tool.claude: Install Claude Code: `curl -fsSL https://claude.ai/install.sh | bash` ...
  2. role.dev: `git worktree add .worktrees/dev` (from the project folder)

Optional:
  - shelf.search: `orai doctor --deep` runs real searches
```

| 표시 | 상태 | 뜻 |
|---|---|---|
| `✓` | healthy | 확인했고 정상 |
| `✗` | blocked, unsupported | 이 기능을 쓸 수 없음. 도구 없음, 로그인 안 됨, 필요한 옵션이 없는 버전 |
| `!` | degraded, not-ready | 일부만 동작하거나 아직 준비 전 |
| `-` | not-configured | `orai.toml`에서 켜지 않은 선택 기능. 문제가 아님 |
| `·` | not-checked | 이번 실행에서 확인하지 않음(`--deep` 필요). 문제도 검증도 아님 |

"Next steps"는 고쳐야 하는 항목이고 "Optional"은 해도 되고 안 해도 되는 항목이다. shelf와 코드 그래프는 선택 기능이므로, 쓰지 않을 프로젝트는 `orai.toml`에서 해당 `[integrations.*]` 표를 지우면 진단에서 빠진다.

**`--deep`이 확인하는 내용 정하기.** 설정이 없으면 `--deep`은 검색 결과가 나오는지만 확인한다. 올바른 문서가 나오는지까지 확인하려면 `orai.toml`에 이 프로젝트 문서에만 있는 사실을 적는다.

```toml
[integrations.shelf.smoke]
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

## shelf (문서 검색)

shelf는 등록한 Markdown 폴더(기본 `docs/`)를 색인해 역할 세션이 찾아볼 수 있게 하는 프로젝트 문서 검색이다. 문서를 옮기거나 정리하지 않는다. 원본은 저장소의 파일이고 shelf는 그 색인이다. 엔진은 QMD이며(현재 유일), 사용자는 `orai shelf`만 쓴다. 같은 안내가 `orai shelf --help`에 있다.

### 처음 준비

1. **QMD 설치**: `npm install -g @tobilu/qmd` (Node 22 이상). 프로젝트에 버전을 고정하려면 `mise use npm:@tobilu/qmd@<버전>`을 쓴다. Orai는 QMD를 설치하지 않는다.
2. **문서 넣기**: `docs/`(또는 `collections`에 적은 폴더)에 Markdown 파일을 둔다. `orai setup`은 폴더가 없으면 시작 페이지 `docs/README.md`를 만든다.
3. **색인 만들기**: `orai shelf init`. QMD가 설치돼 있고 문서가 있으면 `orai setup`이 이 단계를 대신 실행한다.
   - 첫 실행 때 임베딩 모델(약 0.6 GB)을, 첫 실제 검색(`orai doctor --deep` 포함) 때 검색용 모델(약 2 GB)을 `~/.cache/qmd/models`에 받는다. 모델은 프로젝트끼리 공유하므로 한 번만 받는다.
   - 색인과 설정은 이 프로젝트의 `.orai/shelf/`에만 생긴다.
4. **확인**: `orai doctor`에서 `shelf.*` 항목이 `✓`인지 본다. 실제 검색까지 확인하려면 `orai doctor --deep`을 쓴다.

### 설정 (`orai.toml`의 `[integrations.shelf]`)

| 키 | 기본값 | 설명 |
|---|---|---|
| `collections` | `{ docs = "docs" }` | 검색할 묶음. `이름 = "폴더"`(프로젝트 폴더 기준)는 그 폴더 아래의 모든 `.md`를 넣는다. 폴더의 일부만 넣으려면 `이름 = { path = "폴더", pattern = "패턴" }`으로 쓴다 |
| `port` | 프로젝트 경로에서 계산 (18200~18999) | 서버 포트를 고정한다. 주소를 설정 파일에 적어 둘 때([역할 세션 밖에서 쓰기](#역할-세션-밖에서-쓰기))와 계산된 포트를 다른 서버가 쓰고 있을 때 지정한다 |
| `embed_model` | Qwen3-Embedding-0.6B Q8 | 첫 `orai shelf init` 때만 읽는다. 이후 변경은 아래 "모델 변경"을 따른다 |
| `[integrations.shelf.smoke]` | 없음 | `--deep`이 올바른 문서가 검색되는지 확인할 질의. [진단](#진단) 참고 |
| `[integrations.shelf.context]` | 없음 | 폴더별 한 줄 설명. 검색 결과에 `context`로 붙는다. [폴더에 설명 달기](#폴더에-설명-달기-context) 참고 |

`[integrations.shelf]` 표가 없으면 shelf를 쓰지 않는 프로젝트로 본다.

**폴더를 나눠 등록하기.** 묶음을 나누면 에이전트가 검색 범위를 좁힐 수 있다. 한 폴더와 그 하위 폴더를 따로 묶으려면 바깥 폴더에 하위 폴더로 내려가지 않는 패턴을 준다.

```toml
[integrations.shelf.collections]
core = { path = "docs", pattern = "*.md" }   # docs 바로 아래 문서만
adr = "docs/adr"                             # docs/adr 아래 전부 (패턴 생략 = "**/*.md")
product = "docs/product"
```

- 패턴은 그 폴더 기준이다. `*.md`는 그 폴더의 파일만, `**/*.md`는 하위 폴더까지 포함한다.
- 패턴 없이 `docs`와 `docs/adr`을 함께 적으면 `docs/adr`의 문서가 두 번 색인된다. `orai doctor`가 `shelf.collections`로 알려 준다.
- `collections`를 고친 뒤에는 `orai shelf stop && orai shelf refresh`를 실행한다. 새 묶음을 추가하고, 폴더나 패턴이 바뀐 묶음을 다시 등록하고, `orai.toml`에서 지운 묶음을 색인에서 뺀다. 다른 묶음과 모델, DB 파일은 그대로 둔다.
- `recover`는 서버만 다시 켜므로 색인을 고치지 않는다. 선언과 색인이 다르면 무엇이 다른지 알려 주고 위 명령을 안내한다.

### 폴더에 설명 달기 (context)

폴더의 문서가 무엇을 위한 것인지 한 줄로 적어 두면, 그 폴더에서 나온 검색 결과마다 설명이 `context`로 붙는다. 읽는 쪽은 문서를 열기 전에 결정 기록인지 제품 스펙인지 구분할 수 있다. 묶음을 나누지 않고도 문서의 종류를 알려 줄 수 있는 가벼운 방법이다.

```toml
[integrations.shelf.context]
"docs/product" = "화면별 제품 스펙: 사용자 흐름, 동작, 수용 기준"
"docs/adr" = "설계 결정의 배경, 대안, 선택 이유. 지금도 유효한지는 본문에서 확인한다"
"docs/troubleshooting" = "오류 증상, 원인, 재현 조건, 복구 절차"
```

- 키는 프로젝트 폴더 기준 경로다(`"."`은 전체). 묶음에 속하거나 묶음을 포함하는 경로여야 한다. 설명은 한 줄이다.
- 설명은 묶음이 아니라 폴더에 붙는다. 나중에 `docs/adr`을 따로 묶어도 같은 선언이 그대로 적용된다. 바깥 폴더의 설명과 안쪽 폴더의 설명은 바깥 것부터 차례로 함께 붙는다.
- 고친 뒤에는 `orai shelf recover`를 실행한다. 서버를 멈추지 않고 색인도 다시 만들지 않으며, 다음 검색부터 반영된다. `init`과 `refresh`도 같이 적용하므로 새 checkout이나 색인을 다시 만든 뒤에도 설명이 복원된다.
- 표가 있으면 색인의 설명은 표와 같아진다. 표에 없는 설명은 본문을 출력한 뒤 지운다. 표가 없으면 Orai는 색인의 설명을 건드리지 않는다. 빈 표(`[integrations.shelf.context]`만 적음)는 "설명 없음"을 선언한 것이다.
- 선언과 색인이 다르면 `orai doctor`가 `shelf.context`로 알려 준다. 설명을 선언하지 않은 프로젝트에는 이 항목이 나오지 않는다.
- 설명은 어떤 문서가 검색되는지와 순위를 바꾸지 않는다(QMD 2.8.3은 설명을 임베딩과 재순위에 넣지 않고 결과에 붙이기만 한다). 설명의 내용은 프로젝트가 소유한다. 규칙이나 판정 근거가 아니라 문서의 용도와 범위를 알려 주는 글로 쓴다.

**세션이 실제로 보는 것.** 검색 결과 전체가 모델에 전달되는지는 provider마다 다르다([호환성](compatibility.md#shelf-결과가-모델에-전달되는-범위)). 확인한 버전의 Codex는 결과에서 경로·제목·점수만 보고 `context`와 발췌, `get`이 돌려준 본문은 보지 못한다. 그래서 프로젝트 지침은 "shelf로 찾고, 원문은 파일로 읽는다"를 기준으로 한다. 결과의 경로는 `묶음 이름/묶음 안 경로`이므로 묶음의 폴더(`orai.toml`)를 앞에 붙이면 실제 파일이다. 폴더별 설명도 `orai.toml`에 있으므로 어느 provider든 읽을 수 있다.

### 평소 사용

| 상황 | 명령 |
|---|---|
| 문서를 추가하거나 고쳤다, `collections`를 바꿨다 | `orai shelf stop && orai shelf refresh` |
| 폴더 설명(`context`)을 고쳤다 | `orai shelf recover` |
| 재부팅 뒤 서버가 꺼졌다 (`shelf.server`가 `✗`) | `orai shelf recover` |
| 지금 검색이 되는지 확인 | `orai shelf check` |
| 서버 끄기 | `orai shelf stop` |

### 명령

```sh
orai shelf init      # 설정이 없을 때만 생성(기본 모델 Qwen3-Embedding-0.6B Q8), update·embed, 서버 시작, 검증
orai shelf recover   # 기존 설정·DB·모델 보존, 꺼진 서버 시작, 폴더 설명 적용, 검증 (재부팅 후 필요)
orai shelf refresh   # 묶음을 orai.toml에 맞추고 문서·임베딩 갱신 후 시작·검증 (서버가 떠 있으면 거부)
orai shelf check     # 상태 변경 없이 전체 검증 (vector-only → lex+vec → 본문)
orai shelf stop      # 이 프로젝트 index의 서버만 중지
```

- 서버는 `127.0.0.1:<프로젝트 포트>`에서 실행하고, 포트는 `orai doctor`의 `shelf.server.detail.endpoint`로 확인한다. PID와 로그는 `~/.cache/qmd/mcp-orai-<id>.pid|log`에 있다. 다른 프로젝트 서버와 기존 Pockets `8181` 서버는 건드리지 않는다.
- 포트가 다른 index의 서버에 점유돼 있으면 거부하고 그대로 둔다. `orai.toml`의 `integrations.shelf.port`로 다른 포트를 지정한다.
- `init`·`refresh`는 검색이 없는 안전한 시점에 `orai shelf stop`을 먼저 실행한 뒤 수행한다.
- 손상되었거나 다른 프로젝트의 DB를 자동으로 지우지 않는다. 검색 도중 자동 재색인, 모델 다운로드, 서버 재시작도 하지 않는다.
- **모델 변경**은 명시적 재구축 작업이다. 서버를 중지하고, `.orai/shelf/orai-<id>.yml`의 `models.embed`를 바꾼 뒤 `QMD_CONFIG_DIR=.orai/shelf INDEX_PATH=.orai/shelf/index.sqlite qmd --index orai-<id> embed -f`를 실행하고 `orai shelf recover`로 검증한다. 모델 이름, revision, 파일 fingerprint, 메모리 요구량을 기록한다.
- 기본 모델은 한국어·영어 혼합 문서용 초기 후보일 뿐이며 한국어 품질을 보장하지 않는다. `integrations.shelf.smoke`에 이 프로젝트 문서에만 있는 사실을 넣어 recall을 점검한다.
- 현재 한계: 재부팅하면 서버가 내려간다. 첫 버전은 수동 `recover`만 제공하고, 상시 supervisor(launchd)는 필요와 지원 범위를 확인한 뒤 추가한다. CPU fallback(`QMD_FORCE_CPU=1`)은 설치된 바이너리에서 아직 검증하지 않았다.

### 역할 세션 밖에서 쓰기

역할 세션(`orai <역할>`)에는 MCP 서버 `shelf`가 프로세스 인자로 자동 연결된다. 이름은 모든 프로젝트에서 `shelf`이고, 설정 파일에는 아무것도 쓰지 않는다. Desktop 앱이나 그냥 실행한 `claude`·`codex`처럼 역할 세션이 아닌 곳에서 쓰려면 프로젝트의 MCP 설정에 직접 적는다.

```toml
# .codex/config.toml (Codex)
[mcp_servers.shelf]
url = "http://127.0.0.1:18338/mcp"
```

```sh
claude mcp add --transport http --scope project shelf http://127.0.0.1:18338/mcp   # Claude Code, .mcp.json에 기록
```

주소를 파일에 적었으면 `orai.toml`에 포트를 고정한다. 기본 포트는 프로젝트 경로에서 계산하므로 다른 컴퓨터나 다른 경로의 checkout에서는 달라진다.

```toml
[integrations.shelf]
port = 18338
```

`orai doctor`의 `shelf.registration`이 이 두 파일을 읽어 확인한다. Orai는 이 파일들을 고치지 않는다.

Claude Code에 프로젝트 범위로 등록하면 Claude 역할 세션에서는 `shelf`가 두 곳(프로젝트 설정과 Orai가 넘긴 인자)에 있게 된다. 두 주소가 같으면 어느 쪽이 쓰여도 같은 서버이므로, 이 진단이 주소가 같은지를 확인한다. 이름이 겹칠 때 Claude Code가 경고나 오류를 내는지는 실제 세션으로 확인하지 못했다.

| 상태 | 뜻 | 조치 |
|---|---|---|
| `-` not-configured | 두 파일 어디에도 없다. 역할 세션만 쓴다면 문제가 아니다 | 필요하면 Optional에 나오는 주소로 등록한다 |
| `✓` healthy | `shelf`가 이 프로젝트 서버 주소로 적혀 있고 포트가 `orai.toml`에 고정돼 있다 | 없음 |
| `!` degraded | 주소가 다르다, 이 서버가 다른 이름(예전 `wiki-<프로젝트>`)으로 적혀 있다, 파일을 읽을 수 없다, 주소는 맞지만 포트가 고정돼 있지 않다 | Next steps에 고칠 파일과 값이 나온다 |

`--deep`에서는 Claude 역할이 있을 때 `claude mcp get shelf`로 Claude Code의 사용자·로컬 범위도 확인한다. 같은 이름의 서버가 다른 주소로 등록돼 있으면 알린다. 그런 경우 역할 세션이 어느 쪽을 쓰는지는 Claude Code 문서에 나와 있지 않고 직접 확인하지도 못했다.

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
| shelf 서버가 꺼져 있음(`shelf.server` blocked) | `orai shelf recover` |
| shelf가 아직 만들어지지 않음 | QMD 설치 후 `orai shelf init` ([처음 준비](#처음-준비)) |
| 문서를 고쳤는데 검색에 안 나옴(`shelf.index` degraded) | `orai shelf stop && orai shelf refresh` |
| 지운 묶음이 계속 검색되거나 같은 문서가 두 번 나옴(`shelf.collections` degraded) | `orai.toml`의 `collections`를 확인하고 `orai shelf stop && orai shelf refresh` |
| 폴더 설명이 선언과 다름(`shelf.context` degraded) | `orai shelf recover` |
| 역할 밖 설정의 주소가 다름(`shelf.registration` degraded) | Next steps에 나온 파일의 주소·이름을 고치고, `orai.toml`에 `port`를 고정한다 |
| QMD `access denied` | 샌드박스 밖에서 다시 확인한다. 그 결과는 그 환경에만 적용된다 |
| QMD 다른 index 서버 | 그대로 두고 `integrations.shelf.port`를 바꾼다 |

## 검증

```sh
mise run check           # gofmt + go vet + test (CI와 같음)
mise run test            # race 검사기, 빌드한 바이너리를 checkout 밖에서 실행하는 종단 테스트 포함
                         # (AMQ가 설치돼 있으면 메일함 양방향 호환 테스트도 실행)
```

테스트는 실제 역할 세션, 실제 프로젝트 메일함, `8181` 서버를 건드리지 않는다. fake provider와 fixture로 통과한 것을 실제 provider 동작으로 기록하지 않는다.

### 실제 파일럿 (계정·호스트 준비 후)

격리된 작은 파일럿 저장소에서 수행한다. Orai 저장소와 Pockets에서는 하지 않는다.

1. 빈 폴더에서 `mise use --pin github:oXpace/orai`, `orai setup --role pm=codex --role staff=claude`, 첫 커밋, `git worktree add .worktrees/staff`, `orai doctor`.
2. 두 터미널에서 `orai pm`, `orai staff`를 실행한다. Codex `/hooks` 신뢰와 Claude channel에 동의한다.
3. 요청과 회신을 주고받는다(`send` → 알림 → `inbox <ID>` → `reply`). 작업 중 알림, 유휴 알림, 수신자가 꺼져 있을 때의 적재, 재접속 후 알림을 각각 확인한다.
4. 역할을 종료한 뒤 다시 실행해 같은 UUID로 재개되고 메시지를 받는지 확인한다. 중복 실행이 거부되는지 확인한다.
   - 설치 버전에서 아직 확인하지 않은 전제가 있다. Claude `--resume <UUID>`의 SessionStart hook이 같은 `session_id`를 보내야 한다(다르면 재개할 때마다 캡처가 거부된다). Codex SessionStart payload에는 `cwd`와 `transcript_path`가 있어야 하고, `codex resume` 때도 hook이 실행돼야 한다(실행되지 않으면 Codex 알림이 준비 상태가 되지 않는다). 시작과 재개 때 한 번씩 hook 입력을 기록해 확인한다.
5. `orai shelf init` 후 `orai doctor --deep`으로 실제 검색을, `codegraph init` 후 smoke 심볼 조회를 확인한다.

결과는 [이력](history.md)의 검증 기록에 날짜, 버전, 통과·미검증 범위와 함께 남긴다.
