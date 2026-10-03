# Orai

Orai는 Codex와 Claude Code 같은 코딩 에이전트 CLI를 **역할 세션**으로 실행하고, 역할끼리 메시지로 협업하게 하는 프로젝트 하네스다. Go 단일 실행 파일이며 프로젝트마다 설치한다.

- **정확한 재개**: 역할마다 캡처한 대화 UUID로만 재개한다. "가장 최근 대화"를 추측하지 않는다.
- **프로젝트 메일함**: 역할 사이의 메시지를 프로젝트 안의 메일함으로 주고받는다. 디스크 형식이 [AMQ](https://github.com/avivsinai/agent-message-queue)와 같아 `amq`로도 읽을 수 있지만, AMQ 설치는 필요 없다.
- **알림만 전달**: 새 메시지를 파일 변경 이벤트로 즉시 감지해 ID만 Codex queue나 Claude 로컬 MCP channel로 알린다. 메시지를 대신 소비하지 않는다.
- **공통 메시지 액션**: `orai msg inbox`, `orai msg send`, `orai msg reply`
- **프로젝트 wiki와 코드 그래프**: 프로젝트 문서 검색(wiki, 엔진 QMD)과 CodeGraph를 프로젝트별로 격리해 설정·진단·복구한다.
- **한 번에 준비**: 빈 폴더에서 `orai setup` 한 번으로 저장소, 설정, 에이전트 지침, 메일함, wiki, 코드 그래프까지 준비한다.
- **역할은 프로젝트가 정한다**: 이름·개수·provider를 `orai setup --role lead=codex --role dev=claude`처럼 고르고, 나중에도 같은 명령으로 추가한다.
- **진단**: `orai doctor`가 구성요소별 상태와, 남은 조치를 실행할 명령으로 순서대로 보여준다(`--json`은 기계용).

대화는 각 provider가, 역할 조직과 업무 규칙은 각 프로젝트가 소유한다. Orai는 이 책임들을 다시 구현하지 않는다.

### AMQ와의 관계

0.1.0은 [AMQ](https://github.com/avivsinai/agent-message-queue)의 `amq coop`으로 메일함을 만들고 역할을 실행했다. 0.2.0부터는 AMQ를 실행하지 않는다.

| 0.1.0 (AMQ 사용) | 0.2.0~ (내장) |
|---|---|
| `amq coop init`: `.amqrc`, 메일함 생성 | `orai setup`이 메일함 생성 |
| `amq coop exec`: 역할 신원 설정, provider 실행 | `orai run`이 신원·잠금·환경을 정하고 provider 실행 |
| `amq send`·`drain`·`reply` | `orai msg send`·`inbox`·`reply` |
| 2초 주기로 `amq` 호출해 새 메시지 확인 | 파일 변경 이벤트로 즉시 감지 |

메일함 디스크 형식(AMQ schema 1)만 그대로 유지한다. 그래서 AMQ로 쌓인 기존 메일함을 그대로 이어 쓰고, AMQ를 설치했다면 `amq`로 같은 메일함을 읽고 보낼 수 있다. 이 호환성은 CI에서 실제 `amq`와의 양방향 테스트로 확인한다.

## 프로젝트에 추가하기

Orai는 프로젝트마다 설치하고 버전을 고정한다. 준비물은 [mise](https://mise.jdx.dev)와 Git이다. `<버전>`에는 [Releases](https://github.com/oXpace/orai/releases)의 최신 버전을 적는다.

**새 프로젝트**

```sh
mkdir my-app && cd my-app
mise use github:oXpace/orai@<버전>   # 이 프로젝트에 Orai 설치·고정 (mise.toml에 기록)
orai setup --role lead=codex --role dev=claude
```

**이미 있는 프로젝트**

```sh
cd my-app                            # 저장소 루트
mise use github:oXpace/orai@<버전>
orai setup --dry-run --role lead=codex --role dev=claude   # 바뀔 내용만 먼저 확인
orai setup --role lead=codex --role dev=claude
```

기존 파일은 덮어쓰지 않는다. Git 저장소와 커밋은 건드리지 않는다. 충돌이 하나라도 있으면 아무것도 바꾸지 않고 목록을 보여준다.

**setup이 프로젝트에 넣는 것** (새 프로젝트와 기존 프로젝트 모두)

| 파일 | 내용 | 커밋 |
|---|---|---|
| `orai.toml` | 역할과 wiki·코드 그래프 설정. 없을 때만 만들고, 있으면 `--role`로 요청한 역할만 덧붙인다 | 한다 |
| `AGENTS.md` | Orai 설명 블록(역할, 메시지, 진단, 문서 검색, 커밋하지 않는 것). `orai:begin`~`orai:end` 사이만 관리하고 나머지는 그대로 둔다 | 한다 |
| `CLAUDE.md` | 없을 때만 `@AGENTS.md` 한 줄로 만든다 | 한다 |
| `.agents/skills/orai/SKILL.md`, `.claude/skills/orai` | 에이전트가 메시지를 주고받는 방법을 담은 스킬과, Claude Code용 링크 | 한다 |
| `.gitignore` | Orai 블록: `/.orai/`, `/.agent-mail/`, `/.codegraph/`, 역할 작업 폴더 | 한다 |
| `.agents/.gitignore` | Orai 블록: `/roles/` (역할 지침을 저장소에 넣지 않는다) | 한다 |
| `.agents/roles/<역할>.md` | 역할 지침. 이 컴퓨터에서 고쳐 쓴다 | 안 한다 |
| `.orai/`, `.agent-mail/` | 로컬 상태(세션, wiki 색인, 백업)와 메일함 | 안 한다 |
| `docs/README.md` | wiki 폴더가 없을 때만 만드는 시작 페이지 | 한다 |

`AGENTS.md`와 두 `.gitignore`의 원본은 바꾸기 전에 `.orai/backups/`에 보관한다.

**setup이 끝난 뒤**

`setup`은 마지막에 남은 조치를 실행할 명령으로 보여준다(`orai doctor`로 다시 볼 수 있다). 보통 다음이 남는다.

```sh
git add -A && git commit -m "Set up Orai"   # 위 표에서 "한다"인 파일과 mise.toml이 커밋된다
git worktree add .worktrees/dev             # 프로젝트 폴더를 쓰지 않는 역할의 작업 폴더
orai doctor                                 # Status: healthy 확인
```

Codex CLI, Claude Code, QMD, CodeGraph가 없으면 설치 명령이 함께 나온다. wiki와 코드 그래프는 선택 기능이라, 쓰지 않으려면 `orai.toml`에서 해당 `[integrations.*]` 표를 지운다.

**같은 저장소를 받은 사람**은 `mise install` 뒤 `orai setup`을 한 번 실행한다. 버전은 `mise.toml`에, 역할과 설정은 `orai.toml`에 이미 있으므로 그 컴퓨터의 로컬 상태(메일함, 역할 지침의 틀, wiki 색인, 코드 그래프)만 만들어진다. 역할 작업 폴더(`git worktree add`)도 컴퓨터마다 만든다.

## 사용

```sh
# 역할 실행 (역할마다 터미널 하나)
orai lead          # = orai run lead, 기본은 정확한 재개
orai dev --fresh   # 새 대화

# 메시지와 wiki
orai msg send dev --as user --kind todo --body '요청 내용'
orai wiki stop && orai wiki refresh   # docs를 고친 뒤 색인 갱신
orai status && orai doctor
```

- 역할 이름과 개수는 자유다. `--role 이름=codex|claude`를 필요한 만큼 붙인다. 역할 없이 시작했다가 나중에 `orai setup --role reviewer=claude`로 추가해도 된다. `--preset pm-staff`는 `--role pm=codex --role staff=claude`의 줄임이다.
- `setup`은 여러 번 실행해도 안전하다.
- 명령마다 `--help`가 있다. wiki 설정은 `orai wiki --help`, 진단 표시와 종료 코드는 `orai doctor --help`에 정리돼 있다.
- Orai 버전을 올릴 때는 `mise use github:oXpace/orai@<새 버전>` 뒤 `orai setup`을 다시 실행해 생성 파일을 갱신한다.

자세한 절차는 [운영 안내](docs/operations.md)에, 필요한 외부 도구와 확인된 버전은 [호환성](docs/compatibility.md)에 정리돼 있다.

## 상태

`0.2.0` — pre-alpha, Go. 2026-09-25 기준:

| 범위 | 상태 |
|---|---|
| 단위·통합 테스트(race 검사기), 빌드한 바이너리를 checkout 밖에서 실행하는 종단 테스트 | 통과 (`mise run check`, CI macOS·Linux) |
| 메일함의 실제 AMQ 0.80.1 양방향 호환 (Orai ↔ `amq` 전송·수신·답장) | 통과 |
| 호스트 도구 capability·로그인 진단 (Codex 0.157.0, Claude Code 2.1.282) | healthy (`orai doctor`) |
| 이 저장소 문서의 실제 QMD 2.8.3 색인·의미 검색, 서버 다운 감지와 `recover` | 통과 (`orai doctor --deep`) |
| 실제 CodeGraph 1.5.0 색인·심볼 조회 | 통과 |
| Codex↔Claude 실제 요청·회신, 종료 후 동일 UUID 복구 | **미검증** — [실제 파일럿](docs/operations.md#실제-파일럿-계정호스트-준비-후) 대기 |

자세한 기록은 [이력](docs/history.md#검증-기록)에 있다.

## 문서

| 문서 | 내용 |
|---|---|
| [docs/architecture.md](docs/architecture.md) | 책임 경계, 식별·상태 모델, 세션·알림·연동 흐름, 진단 모델 |
| [docs/operations.md](docs/operations.md) | 설치, 초기화, 실행·중지, 진단, QMD/CodeGraph 운영, 복구, 검증 |
| [docs/compatibility.md](docs/compatibility.md) | 지원 플랫폼, 도구 버전·capability·설치 출처, 라이선스 |
| [docs/history.md](docs/history.md) | Pockets 추출 원천, 알려진 장애, 변환 내역, 검증 기록 |
| [docs/release.md](docs/release.md) | 배포 단계, 버전 정책, 릴리스 점검 |
| [AGENTS.md](AGENTS.md) | 이 저장소에서 일하는 에이전트를 위한 개발 지침 |

## 라이선스

[MIT](LICENSE) © 2026 oXpace

Orai는 Codex CLI, Claude Code, QMD, CodeGraph를 번들하거나 재배포하지 않는다. 사용자가 설치한 실행 파일을 별도 프로세스로 호출할 뿐이며, 각 도구의 라이선스와 약관(Claude Code는 Anthropic 상용 약관)은 사용자에게 그대로 적용된다.
