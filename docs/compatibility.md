# 호환성

이 문서는 지원 플랫폼, 의존 도구의 버전과 capability, 설치 출처를 소유한다. 표의 "확인" 값은 적힌 날짜에 해당 호스트에서 관찰한 사실이다. 이후 버전에서도 동작한다는 보장이 아니다. 버전을 바꿀 때는 `orai doctor`와 [운영 안내](operations.md)의 검증을 다시 수행하고 이 표를 갱신한다.

## 플랫폼

| 대상 | 범위 |
|---|---|
| macOS arm64 | 개발·검증 환경. 코어 테스트와 실제 provider 실행 대상 |
| Linux x64/arm64 | 코어 테스트(CI) 대상. 실제 Codex/Claude 실행은 미검증 |
| Windows | 지원하지 않음 (`fcntl`, POSIX 신호, 파일 권한 모델에 의존) |

Python은 3.11 이상(`tomllib`, `datetime.UTC`)을 지원한다. 개발 기본값은 `mise.toml`에 pin한 3.14.7이고, CI는 3.11과 3.14.7에서 모두 실행한다. Orai 런타임은 표준 라이브러리만 사용한다.

## 개발 도구 (mise 관리)

| 도구 | 버전 | 출처 |
|---|---|---|
| mise | 2026.9.13 이상 (`min_version` 2026.9.0) | 사용자 설치 |
| Python | 3.14.7 | `core:python` (python-build-standalone), `mise.lock` |
| uv | 0.12.18 | `aqua:astral-sh/uv`, `mise.lock` |
| Orai (소비 프로젝트) | 프로젝트 `mise.toml` 고정 | `pypi:oXpace/orai` (mise `pypi` backend, uv 사용). 로컬 `git+file://` 원천은 지원하지 않음(mise 2026.9.13 확인) |
| pytest, ruff | `uv.lock` | PyPI (`dependency-groups.dev`) |

Python 의존성의 정본은 `pyproject.toml`과 `uv.lock`이다. mise는 런타임과 task만 관리한다.

## 외부 도구 (사용자 설치, Orai는 설치·업그레이드하지 않음)

2026-09-25 macOS 27.0 arm64 호스트에서 확인했다.

| 도구 | 확인 버전 | 설치 출처(확인) | Orai가 쓰는 capability | doctor 검사 |
|---|---|---|---|---|
| AMQ | 0.80.1 | Homebrew `avivsinai/tap/amq` | `env --json`, `coop exec --no-wake --no-init --named=false --root --me`, `list --new`, `drain --include-body --limit`, `read --id`, `send`, `reply`, `init --force`(handle 추가 시 미처리 메일 보존 확인) | 도움말의 플래그 |
| Codex CLI | 0.157.0 | Homebrew cask `codex` | `resume <UUID>`, `queue --thread --message`, `-c hooks.SessionStart`, `-c mcp_servers.*`, `--add-dir` | 도움말 + `codex login status` |
| Claude Code | 2.1.282 | 네이티브 설치 (`~/.local/share/claude/versions`) | `--session-id`, `--resume`, `--settings`, `--mcp-config`, `--dangerously-load-development-channels server:orai`, `--name`, `--effort` | 도움말 + `claude auth status` (channel 플래그는 도움말에 없어 실행 시 확인) |
| QMD | 2.8.3 | npm `@tobilu/qmd` (mise Node 24.21.0 전역) | `--index`, `QMD_CONFIG_DIR`/`INDEX_PATH`, `collection show`, `update`, `embed`, `mcp --http --daemon --host --port`, `mcp stop`, MCP `status`/`query`/`get` | 포트·MCP·식별·색인, `--deep`에서 검색 |
| CodeGraph | 1.5.0 (upstream 최신 1.6.0) | 번들 설치 `~/.codegraph/versions/v1.5.0` (npm 설치 아님) | `status --json`, `query --json --path`, `init`, `sync`, `install --print-config`, `--location local` | `status --json`, `--deep`에서 심볼 조회 |
| Git | 2.55.0 | Homebrew | `worktree list --porcelain`, `rev-parse --show-toplevel` | 없음 |

과거 기록(Pockets 문서): AMQ 0.77.3, Codex 0.154.0, Claude Code 2.1.268. 현재 지원 버전과 같다고 가정하지 않는다.

### 설치 방식 선택 근거

- **AMQ**: mise registry에 없다. upstream이 제공하는 Homebrew tap을 기준으로 한다. 전역 설치이므로 Orai는 버전을 바꾸지 않는다.
- **Codex / Claude / CodeGraph**: mise registry에 aqua backend(`aqua:openai/codex`, `aqua:anthropics/claude-code`, `aqua:colbymchenry/codegraph`)가 있다. 다만 사용 중인 전역 설치를 가리지 않도록 이 저장소의 `mise.toml`에는 넣지 않는다. 버전 고정이 필요한 파일럿 프로젝트에서는 해당 프로젝트의 `mise.toml`에 pin하는 방식을 권장한다.
- **CodeGraph 이름 주의**: upstream은 `colbymchenry/codegraph`이고 npm 이름은 `@colbymchenry/codegraph`다. 이름이 같은 다른 도구를 설치하지 않는다.
- **QMD**: npm 패키지이며 네이티브 빌드 스크립트(`node-llama-cpp` 등)를 실행한다. 일회성 전역 `npm install -g`에만 기대지 않는다. 고정이 필요하면 `mise use npm:@tobilu/qmd@2.8.3`처럼 버전을 지정해 설치하고, 설치 후 `orai doctor`로 확인한다.

## 라이선스

Orai는 [MIT](../LICENSE)(© 2026 oXpace)다. 아래 도구는 번들하지 않고 사용자가 설치한 실행 파일을 별도 프로세스로 호출하므로, Orai의 라이선스를 제약하지 않는다(2026-09-25 각 저장소의 라이선스 확인).

| 대상 | 라이선스 | 비고 |
|---|---|---|
| AMQ | MIT | 외부 실행 파일 |
| QMD (내부 node-llama-cpp MIT) | MIT | 외부 실행 파일 (wiki 엔진) |
| CodeGraph | MIT | 외부 실행 파일 |
| Codex CLI | Apache-2.0 | 외부 실행 파일 |
| Claude Code | 상용 약관 (Anthropic Commercial Terms, 오픈소스 아님) | 사용자가 설치·로그인한 것을 실행만 한다. 번들·재배포하지 않는다 |
| Qwen3-Embedding-0.6B (wiki 기본 모델) | Apache-2.0 | QMD가 사용자 캐시에 받는다. Orai는 배포하지 않는다 |
| mise, pytest, ruff / uv | MIT / MIT·Apache-2.0 | 개발 도구 |
| Pockets 추출 원천 | 작성자(Ox) 소유, 외부 기여 없음 | MIT로 공개. 가져온 파일의 커밋 작성자는 모두 `Ox`이며 AI 공동 작성 표기만 있음 |
