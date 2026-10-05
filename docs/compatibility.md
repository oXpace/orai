# 호환성

이 문서는 지원 플랫폼, 의존 도구의 버전과 capability, 설치 출처를 소유한다. 표의 "확인" 값은 적힌 날짜에 해당 호스트에서 관찰한 사실이다. 이후 버전에서도 동작한다는 보장이 아니다. 버전을 바꿀 때는 `orai doctor`와 [운영 안내](operations.md)의 검증을 다시 수행하고 이 표를 갱신한다.

## 플랫폼

| 대상 | 범위 |
|---|---|
| macOS arm64 | 개발·검증 환경. 코어 테스트와 실제 provider 실행 대상 |
| Linux x64/arm64 | 코어 테스트(CI) 대상. 실제 Codex/Claude 실행은 미검증 |
| Windows | 지원하지 않음 (`flock`, POSIX 신호, 파일 권한, `link(2)` 배달에 의존) |

Orai는 Go 단일 실행 파일이다. 릴리스는 darwin/linux × arm64/amd64로 빌드하며(`CGO_ENABLED=0`), 실행에 별도 런타임이 필요 없다. 외부 Go 의존성은 `github.com/BurntSushi/toml`(설정 파싱)과 `github.com/fsnotify/fsnotify`(메일함 변경 감시) 두 개다.

## 개발 도구 (mise 관리)

| 도구 | 버전 | 출처 |
|---|---|---|
| mise | 2026.10.0에서 확인 (`min_version` 2026.9.0) | 사용자 설치 |
| Go | 1.27.1 | `core:go`, `mise.lock` |
| Orai (소비 프로젝트) | 프로젝트 `mise.toml` 고정 | `github:oXpace/orai` (mise github backend, GitHub Release의 바이너리) |

Go 의존성의 정본은 `go.mod`와 `go.sum`이다. mise는 Go 버전과 task만 관리한다. 0.1.0은 Python 패키지(`pypi:oXpace/orai`)였고 0.2.0부터 Go 바이너리다.

## 외부 도구 (사용자 설치, Orai는 설치·업그레이드하지 않음)

`orai doctor`가 없는 도구마다 아래 설치 명령을 안내한다.

| 도구 | 설치 | 필요한 경우 |
|---|---|---|
| Codex CLI | `npm install -g @openai/codex` 또는 `brew install --cask codex`, 이후 `codex login` | provider가 `codex`인 역할이 있을 때 |
| Claude Code | `curl -fsSL https://claude.ai/install.sh \| bash` 또는 `npm install -g @anthropic-ai/claude-code`, 이후 `claude auth login` | provider가 `claude`인 역할이 있을 때 |
| QMD | `npm install -g @tobilu/qmd` (Node 22 이상) | `[integrations.shelf]`를 쓸 때 (선택) |
| CodeGraph | `npm install -g @colbymchenry/codegraph` | `[integrations.codegraph]`를 쓸 때 (선택) |

2026-10-05 macOS 27.0 arm64 호스트에서 확인했다.

| 도구 | 확인 버전 | 설치 출처(확인) | Orai가 쓰는 capability | doctor 검사 |
|---|---|---|---|---|
| Codex CLI | 0.160.0 | Homebrew cask `codex` | `resume <UUID>`, `queue --thread --message`, `-c hooks.SessionStart`, `-c mcp_servers.*`, `--add-dir` | 도움말 + `codex login status` |
| Claude Code | 2.1.289 | 네이티브 설치 (`~/.local/share/claude/versions`) | `--session-id`, `--resume`, `--settings`, `--mcp-config`, `--dangerously-load-development-channels server:orai`, `--name`, `--effort` | 도움말 + `claude auth status` (channel 플래그는 도움말에 없어 실행 시 확인). `--deep`은 `claude mcp get shelf`의 출력(`URL:` 줄)으로 같은 이름의 다른 서버를 찾는다 |
| QMD | 2.8.3 | npm `@tobilu/qmd` (mise Node 24.21.0 전역) | `--index`, `QMD_CONFIG_DIR`/`INDEX_PATH`, `collection show`, `update`, `embed`, `mcp --http --daemon --host --port`, `mcp stop`, MCP `status`/`query`/`get`, `context list`/`add`/`rm` | 포트·MCP·식별·색인·폴더 설명, `--deep`에서 검색 |
| CodeGraph | 1.6.0 | 번들 설치 `~/.codegraph/versions/v1.6.0` (npm 설치 아님) | `status --json`, `query --json --path`, `init`, `sync`, `install --print-config`, `--location local` | `status --json`, `--deep`에서 심볼 조회 |
| Git | 2.56.0 | Homebrew | `worktree list --porcelain`, `rev-parse --show-toplevel` | 없음 |

### shelf 결과가 모델에 전달되는 범위

QMD의 MCP 응답은 검색 결과를 둘로 나눠 싣는다. 글(`content[].text`)에는 문서 ID·점수·경로·제목만 있고, 구조화된 부분(`structuredContent.results[]`)에 `context`·줄 번호·발췌가 있다. `get`은 본문을 리소스(`type: "resource"`)로 돌려준다. 이 가운데 무엇을 모델에 넘기는지는 provider가 정한다. 2026-10-05에 임시 프로젝트의 서버를 `claude -p`와 `codex exec`에 연결해, 모델이 받은 내용을 그대로 적게 하는 방식으로 확인했다.

| provider | 검색(`query`) | 본문(`get`, `multi_get`) |
|---|---|---|
| Claude Code 2.1.289 | 구조화된 결과 전체를 본다: 경로, 제목, 점수, `context`, 줄 번호, 발췌 | 본문을 본다 |
| Codex CLI 0.160.0 | 글만 본다: 문서 ID, 점수, 경로, 제목. `context`와 발췌는 보지 못한다 | 아무것도 보지 못한다(세 번 실행해 같았다). `codex exec --json`의 이벤트에는 서버가 보낸 본문과 구조화된 결과가 모두 들어 있으므로, 서버가 아니라 Codex가 모델에 넘기는 단계에서 빠진다 |

Orai는 서버와 provider 사이에 끼지 않으므로 이 차이를 메우지 못한다. Codex 역할에서 shelf는 "어느 파일인지 찾는" 도구로 동작하고, 본문과 폴더 설명은 파일과 `orai.toml`에서 읽는다. 프로젝트 지침(`AGENTS.md`의 Orai 블록)이 이 순서를 안내한다. 역할 세션(TUI)에서 같은지는 확인하지 않았다.

과거 기록(Pockets 문서): AMQ 0.77.3, Codex 0.154.0, Claude Code 2.1.268. 현재 지원 버전과 같다고 가정하지 않는다.

### AMQ (선택, 필요 없음)

0.2.0부터 Orai는 메일함을 직접 관리하므로 AMQ가 필요 없다. 디스크 형식은 AMQ schema 1과 같다. 그래서 AMQ를 설치했다면 `amq`로 같은 메일함을 읽고 쓸 수 있고, 역할 세션에는 이를 위한 `AM_ROOT`·`AM_ME`·`AM_SESSION`이 설정된다. 호환성은 AMQ 0.85.0(Homebrew `avivsinai/tap/amq`, 이전에는 0.80.1)과 양방향 테스트로 확인했다(Orai → `amq drain`·`amq reply`, `amq send` → Orai 수신·답장). AMQ가 이 형식을 바꾸면 이 호환성은 깨질 수 있다.

### 설치 방식 선택 근거

- **Codex / Claude / CodeGraph**: mise registry에 aqua backend(`aqua:openai/codex`, `aqua:anthropics/claude-code`, `aqua:colbymchenry/codegraph`)가 있다. 다만 사용 중인 전역 설치를 가리지 않도록 이 저장소의 `mise.toml`에는 넣지 않는다. 버전 고정이 필요한 파일럿 프로젝트에서는 해당 프로젝트의 `mise.toml`에 pin하는 방식을 권장한다.
- **CodeGraph 이름 주의**: upstream은 `colbymchenry/codegraph`이고 npm 이름은 `@colbymchenry/codegraph`다. 이름이 같은 다른 도구를 설치하지 않는다.
- **QMD**: npm 패키지이며 네이티브 빌드 스크립트(`node-llama-cpp` 등)를 실행한다. 일회성 전역 `npm install -g`에만 기대지 않는다. 고정이 필요하면 `mise use npm:@tobilu/qmd@2.8.3`처럼 버전을 지정해 설치하고, 설치 후 `orai doctor`로 확인한다.

## 라이선스

Orai는 [MIT](../LICENSE)(© 2026 oXpace)다. 외부 도구는 번들하지 않고 사용자가 설치한 실행 파일을 별도 프로세스로 호출한다. 바이너리에 포함되는 Go 의존성은 MIT·BSD 계열이다. 어느 쪽도 Orai의 라이선스를 제약하지 않는다(2026-09-25 각 저장소의 라이선스 확인).

| 대상 | 라이선스 | 비고 |
|---|---|---|
| AMQ | MIT | 메일함 디스크 형식만 호환. 코드나 실행 파일을 포함하지 않음 |
| QMD (내부 node-llama-cpp MIT) | MIT | 외부 실행 파일 (shelf 엔진) |
| CodeGraph | MIT | 외부 실행 파일 |
| Codex CLI | Apache-2.0 | 외부 실행 파일 |
| Claude Code | 상용 약관 (Anthropic Commercial Terms, 오픈소스 아님) | 사용자가 설치·로그인한 것을 실행만 한다. 번들·재배포하지 않는다 |
| Qwen3-Embedding-0.6B (shelf 기본 모델) | Apache-2.0 | QMD가 사용자 캐시에 받는다. Orai는 배포하지 않는다 |
| BurntSushi/toml / fsnotify, golang.org/x/sys | MIT / BSD-3-Clause | 바이너리에 포함되는 Go 의존성 |
| mise, Go | MIT / BSD-3-Clause | 개발 도구 |
| Pockets 추출 원천 | 작성자(Ox) 소유, 외부 기여 없음 | MIT로 공개. 가져온 파일의 커밋 작성자는 모두 `Ox`이며 AI 공동 작성 표기만 있음 |
