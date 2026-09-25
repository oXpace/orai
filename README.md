# Orai

Orai는 Codex와 Claude Code 같은 코딩 에이전트 CLI를 **역할 세션**으로 실행하고, 역할끼리 메시지로 협업하게 하는 프로젝트 하네스다. Go 단일 실행 파일이며 프로젝트마다 설치한다.

- **정확한 재개**: 역할마다 캡처한 대화 UUID로만 재개한다. "가장 최근 대화"를 추측하지 않는다.
- **프로젝트 메일함**: 역할 사이의 메시지를 프로젝트 안의 메일함으로 주고받는다. 디스크 형식이 [AMQ](https://github.com/avivsinai/agent-message-queue)와 같아 `amq`로도 읽을 수 있지만, AMQ 설치는 필요 없다.
- **알림만 전달**: 새 메시지를 파일 변경 이벤트로 즉시 감지해 ID만 Codex queue나 Claude 로컬 MCP channel로 알린다. 메시지를 대신 소비하지 않는다.
- **공통 메시지 액션**: `orai msg inbox`, `orai msg send`, `orai msg reply`
- **프로젝트 wiki와 코드 그래프**: 프로젝트 문서 검색(wiki, 엔진 QMD)과 CodeGraph를 프로젝트별로 격리해 설정·진단·복구한다.
- **한 번에 준비**: 빈 폴더에서 `orai setup` 한 번으로 저장소, 설정, 에이전트 지침, 메일함, wiki, 코드 그래프까지 준비한다.
- **진단**: `orai doctor`가 구성요소별 상태와 안전한 다음 조치를 JSON으로 보고한다.

대화는 각 provider가, 역할 조직과 업무 규칙은 각 프로젝트가 소유한다. Orai는 이 책임들을 다시 구현하지 않는다.

## 빠른 시작

Orai는 프로젝트마다 설치하고 버전을 고정한다([운영 안내](docs/operations.md#설치)).

```sh
mkdir my-app && cd my-app
mise use github:oXpace/orai@<버전>   # 이 프로젝트에 Orai 설치·고정 (mise.toml)
orai setup --preset pm-staff          # 저장소(trunk)·설정·지침·메일함·wiki·코드 그래프·진단

# 역할 실행 (역할마다 터미널 하나)
orai pm            # = orai run pm, 기본은 정확한 재개
orai staff --fresh # 새 대화

# 메시지와 wiki
orai msg send staff --as user --kind todo --body '요청 내용'
orai wiki refresh  # docs를 고친 뒤 색인 갱신
orai status && orai doctor
```

`--dry-run`은 바꿀 내용만 보여준다. `setup`은 여러 번 실행해도 안전하다.

필요한 외부 도구와 확인된 버전은 [호환성](docs/compatibility.md)에 정리돼 있다.

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
