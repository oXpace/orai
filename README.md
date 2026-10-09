# Orai

[![release](https://img.shields.io/github/v/release/oXpace/orai)](https://github.com/oXpace/orai/releases/latest)
[![ci](https://github.com/oXpace/orai/actions/workflows/ci.yml/badge.svg?branch=trunk)](https://github.com/oXpace/orai/actions/workflows/ci.yml)
[![license](https://img.shields.io/github/license/oXpace/orai)](LICENSE)

Orai는 Codex와 Claude Code 같은 코딩 에이전트 CLI를 **역할 세션**으로 실행하고, 역할끼리 메시지로 협업하게 하는 프로젝트 하네스다. Go 단일 실행 파일이며 프로젝트마다 설치한다.

- **정확한 재개**: 역할마다 캡처한 대화 UUID로만 재개한다. "가장 최근 대화"를 추측하지 않는다.
- **프로젝트 메일함**: 역할 사이의 메시지를 프로젝트 안의 메일함으로 주고받는다. 디스크 형식이 [AMQ](https://github.com/avivsinai/agent-message-queue)와 같아 `amq`로도 읽을 수 있지만, AMQ 설치는 필요 없다.
- **알림만 전달**: 새 메시지를 파일 변경 이벤트로 즉시 감지해 ID만 Codex queue나 Claude 로컬 MCP channel로 알린다. 메시지를 대신 소비하지 않는다.
- **공통 메시지 액션**: `orai msg inbox`, `orai msg send`, `orai msg reply`
- **문서 검색(shelf)과 코드 그래프**: 프로젝트 문서 폴더를 묶음별로 등록해 역할 세션에서 검색하는 shelf(엔진 QMD)와 CodeGraph를 프로젝트별로 격리해 설정·진단·복구한다.
- **한 번에 준비**: 빈 폴더에서 `orai setup` 한 번으로 저장소, 설정, 에이전트 지침, 메일함, shelf, 코드 그래프까지 준비한다.
- **역할은 프로젝트가 정한다**: 이름·개수·provider를 `orai setup --role lead=codex --role dev=claude`처럼 고르고, 나중에도 같은 명령으로 추가한다.
- **진단**: `orai doctor`가 구성요소별 상태와, 남은 조치를 실행할 명령으로 순서대로 보여준다(`--json`은 기계용).

대화는 각 provider가, 역할 조직과 업무 규칙은 각 프로젝트가 소유한다. Orai는 이 책임들을 다시 구현하지 않는다.

### AMQ와의 관계

Orai는 [AMQ](https://github.com/avivsinai/agent-message-queue)를 실행하지 않으며 설치할 필요도 없다. 메일함의 디스크 형식(AMQ schema 1)만 같게 유지한다. 그래서 AMQ로 쌓인 메일함을 그대로 이어 쓸 수 있고, AMQ를 설치했다면 `amq`로 같은 메일함을 읽고 보낼 수 있다. 이 호환성은 테스트에서 실제 `amq`와 메시지를 주고받아 확인한다.

## 프로젝트에 추가하기

Orai는 프로젝트마다 설치하고 버전을 고정한다. 준비물은 [mise](https://mise.jdx.dev)와 Git이다. 아래 명령은 최신 릴리스를 받아 그 버전을 프로젝트의 `mise.toml`에 적는다. 특정 버전을 쓰려면 `mise use github:oXpace/orai@0.6.0`처럼 `v` 없이 번호를 붙인다. 막 나온 릴리스는 mise의 버전 목록에 늦게 나타날 수 있다. `mise.toml`에 적힌 버전이 위 배지보다 낮으면 번호를 직접 붙여 다시 실행한다.

**새 프로젝트**

```sh
mkdir my-app && cd my-app
mise use --pin github:oXpace/orai    # 이 프로젝트에 Orai 설치·고정 (mise.toml에 기록)
orai setup --role lead=codex --role dev=claude
```

**이미 있는 프로젝트**

```sh
cd my-app                            # 저장소 루트
mise use --pin github:oXpace/orai
orai setup --dry-run --role lead=codex --role dev=claude   # 바뀔 내용만 먼저 확인
orai setup --role lead=codex --role dev=claude
```

기존 파일은 덮어쓰지 않는다. Git 저장소와 커밋은 건드리지 않는다. 충돌이 하나라도 있으면 아무것도 바꾸지 않고 목록을 보여준다.

**setup이 프로젝트에 넣는 것** (새 프로젝트와 기존 프로젝트 모두)

| 파일 | 내용 | 커밋 |
|---|---|---|
| `orai.toml` | 프로젝트가 공유하는 설정: 역할과 shelf·코드 그래프. 없을 때만 만들고, 있으면 `--role`로 요청한 역할만 덧붙인다 | 한다 |
| `orai.local.toml` | 이 컴퓨터만의 설정(역할의 모델, 포트, 나만 쓰는 역할). 직접 만들 때만 있고 `orai.toml` 위에 키 단위로 겹친다 | 안 한다 |
| `AGENTS.md` | Orai 블록: 목적, 할 일별 도구와 스킬 진입점, 설정 위치. `orai:begin`~`orai:end` 사이만 관리하고 나머지는 그대로 둔다 | 한다 |
| `CLAUDE.md` | 없을 때만 `@AGENTS.md` 한 줄로 만든다 | 한다 |
| `.agents/skills/orai/`, `.claude/skills/orai` | Orai 명령의 실행법을 담은 스킬(`SKILL.md`: 메시지, `references/`: 문서·코드 찾기, 상태 확인·복구)과 Claude Code용 링크 | 한다 |
| `.gitignore` | Orai 블록: `/.orai/`, `/orai.local.toml`, `/.agent-mail/`, `/.codegraph/`, 역할 작업 폴더 | 한다 |
| `.agents/.gitignore` | Orai 블록: `/roles/` (역할 지침을 저장소에 넣지 않는다) | 한다 |
| `.agents/roles/<역할>.md` | 역할 지침. 이 컴퓨터에서 고쳐 쓴다 | 안 한다 |
| `.orai/`, `.agent-mail/` | 로컬 상태(세션, shelf 색인, 백업)와 메일함 | 안 한다 |
| `docs/README.md` | shelf에 등록한 폴더가 없을 때만 만드는 시작 페이지 | 한다 |

`AGENTS.md`와 두 `.gitignore`의 원본은 바꾸기 전에 `.orai/backups/`에 보관한다.

**setup이 끝난 뒤**

`setup`은 마지막에 남은 조치를 실행할 명령으로 보여준다(`orai doctor`로 다시 볼 수 있다). 보통 다음이 남는다.

```sh
git add -A && git commit -m "Set up Orai"   # 위 표에서 "한다"인 파일과 mise.toml이 커밋된다
git worktree add .worktrees/dev             # 프로젝트 폴더를 쓰지 않는 역할의 작업 폴더
orai doctor                                 # Status: healthy 확인
```

Codex CLI, Claude Code, QMD, CodeGraph가 없으면 설치 명령이 함께 나온다. shelf와 코드 그래프는 선택 기능이라, 쓰지 않으려면 `orai.toml`에서 해당 `[integrations.*]` 표를 지운다.

**같은 저장소를 받은 사람**은 `mise install` 뒤 `orai setup`을 한 번 실행한다. 버전은 `mise.toml`에, 역할과 설정은 `orai.toml`에 이미 있으므로 그 컴퓨터의 로컬 상태(메일함, 역할 지침의 틀, shelf 색인, 코드 그래프)만 만들어진다. 역할을 실행할 사람은 역할 작업 폴더(`git worktree add`)도 컴퓨터마다 만든다. 역할을 쓰지 않아도 문서 검색과 진단은 그대로 쓴다. 모델이나 포트처럼 컴퓨터마다 달라야 하는 값은 `orai.local.toml`에 적는다([운영 안내](docs/operations.md#공유-설정과-로컬-설정)).

## 사용

```sh
# 역할 실행 (역할마다 터미널 하나)
orai lead          # = orai run lead, 기본은 정확한 재개
orai dev --fresh   # 새 대화

# 메시지와 shelf
orai msg send dev --as user --kind todo --body '요청 내용'
orai shelf sync                         # docs를 고친 뒤 색인 갱신 (서버는 계속 돈다)
orai status && orai doctor
```

- 역할 이름과 개수는 자유다. `--role 이름=codex|claude`를 필요한 만큼 붙인다. 역할 없이 시작했다가 나중에 `orai setup --role reviewer=claude`로 추가해도 된다. `--preset manager-engineer`는 `--role manager=codex --role engineer=claude`의 줄임이고, `--preset manager-engineer:manager=pm,engineer=staff`처럼 역할 이름을 바꿔 쓸 수 있다.
- `setup`은 여러 번 실행해도 안전하다.
- shelf는 `orai.toml`의 `[integrations.shelf]`에 등록한 폴더를 검색한다. `adr = "docs/adr"`처럼 묶음을 나눠 등록할 수 있고, 폴더마다 한 줄 설명(`context`)을 달아 검색 결과에 붙일 수 있다. 역할 세션에는 `shelf`라는 MCP 서버로 연결된다. Desktop처럼 역할 세션 밖에서 쓰는 방법은 [운영 안내](docs/operations.md#역할-세션-밖에서-쓰기)에 있다.
- 명령마다 `--help`가 있다. shelf 설정은 `orai shelf --help`, 진단 표시와 종료 코드는 `orai doctor --help`에 정리돼 있다.

자세한 절차는 [운영 안내](docs/operations.md)에, 필요한 외부 도구와 확인된 버전은 [호환성](docs/compatibility.md)에 정리돼 있다.

## 버전 올리기

```sh
mise use --pin github:oXpace/orai    # 최신 릴리스로 고정을 바꾼다
orai setup                           # 스킬, AGENTS.md 블록, .gitignore 블록을 새 버전에 맞게 갱신
orai doctor
```

저장된 대화와 메일함은 그대로 남고, 역할 세션은 다시 열면 된다. 버전마다 따로 해야 할 일은 [운영 안내의 마이그레이션](docs/operations.md#버전-올리기와-마이그레이션)에, 바뀐 내용은 [릴리스 노트](https://github.com/oXpace/orai/releases)에 있다.

## 상태

pre-alpha다. 최신 버전은 위 배지와 [Releases](https://github.com/oXpace/orai/releases)에서 확인한다.

| 범위 | 상태 |
|---|---|
| 단위·통합 테스트(race 검사기), 빌드한 바이너리를 checkout 밖에서 실행하는 종단 테스트 | 통과 (`mise run check`, CI는 위 배지) |
| 메일함의 실제 AMQ 양방향 호환 (Orai ↔ `amq` 전송·수신·답장) | 통과 |
| 호스트 도구(Codex, Claude Code) capability·로그인 진단 | 통과 (`orai doctor`) |
| 이 저장소 문서의 실제 shelf 색인·의미 검색, 서버 다운 감지와 `recover` | 통과 (`orai doctor --deep`) |
| 실제 CodeGraph 색인·심볼 조회 | 통과 |
| Codex↔Claude 실제 요청·회신, 종료 후 동일 대화 복구 | **미검증** — [실제 파일럿](docs/operations.md#실제-파일럿-계정호스트-준비-후) 대기 |

확인한 도구 버전은 [호환성](docs/compatibility.md)에, 날짜별 검증 기록은 [이력](docs/history.md#검증-기록)에 있다.

## 문서

| 문서 | 내용 |
|---|---|
| [docs/architecture.md](docs/architecture.md) | 책임 경계, 식별·상태 모델, 세션·알림·연동 흐름, 진단 모델 |
| [docs/operations.md](docs/operations.md) | 설치, 버전 올리기와 마이그레이션, 초기화, 실행·중지, 진단, shelf·CodeGraph 운영, 복구, 검증 |
| [docs/compatibility.md](docs/compatibility.md) | 지원 플랫폼, 도구 버전·capability·설치 출처, 라이선스 |
| [docs/history.md](docs/history.md) | Pockets 추출 원천, 알려진 장애, 변환 내역, 검증 기록 |
| [docs/release.md](docs/release.md) | 배포 단계, 버전 정책, 릴리스 점검 |
| [AGENTS.md](AGENTS.md) | 이 저장소에서 일하는 에이전트를 위한 개발 지침 |

## 라이선스

[MIT](LICENSE) © 2026 oXpace

Orai는 Codex CLI, Claude Code, QMD, CodeGraph를 번들하거나 재배포하지 않는다. 사용자가 설치한 실행 파일을 별도 프로세스로 호출할 뿐이며, 각 도구의 라이선스와 약관(Claude Code는 Anthropic 상용 약관)은 사용자에게 그대로 적용된다.
