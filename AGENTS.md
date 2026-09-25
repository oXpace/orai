# AGENTS.md

이 문서는 Orai 저장소에서 개발하는 에이전트의 작업 원칙과 도구 진입점을 소유한다. 제품 설계는 [docs/architecture.md](docs/architecture.md)가, 운영 절차는 [docs/operations.md](docs/operations.md)가 소유한다. 같은 규칙을 여러 문서에 중복하지 않는다.

## 원칙

1. Orai의 경계를 지킨다. 대화는 provider, 역할 조직과 업무 규칙은 소비 프로젝트가 소유한다. 메일함은 Orai가 소유하되 디스크 형식은 AMQ schema 1과 호환되게 유지한다(`internal/mail`의 교차 호환 테스트가 이를 지킨다).
2. 플러그인 프레임워크, 자체 범용 MCP 서버, GUI, 클라우드 스케줄러를 먼저 만들지 않는다. CLI와 진단을 먼저 완성한다.
3. 기존 계약(정확한 재개, nonce 기반 캡처, 알림은 ID만, 소비하지 않는 notifier, 대화형 입력 즉시 거부, 역할 신원 고정)을 바꿀 때는 먼저 계약 테스트로 동등성을 보인다.
4. 실패를 성공으로 보고하지 않는다. 빈 검색, 도구 실패, 샌드박스 접근 거부를 대상이 없다는 근거나 서버 다운의 근거로 삼지 않는다.
5. 테스트는 실제 역할 세션, 실제 프로젝트 메일함, 사용자의 전역 설정, 다른 프로젝트의 wiki 서버(`8181` 포함)를 건드리지 않는다.
6. Pockets(`/Users/jason/Developments/Pockets`)는 읽기 전용 참고 대상이다. 원천을 인용할 때는 `git show <sha>:<경로>`로 확인하고 [docs/history.md](docs/history.md)에 기록한다.
7. 되돌리기 어려운 작업(원격 push, 배포, 전역 도구 설치·업그레이드, 다른 프로젝트 변경)은 사용자 확인 후에만 수행한다.

## 개발 환경

| 작업 | 명령 |
|---|---|
| 도구 설치 | `mise install` (Go pin, `mise.lock`) |
| 의존성 | `mise run setup` (`go mod download`) |
| 정적 검사·포맷 | `mise run lint` (gofmt, go vet), `mise run fmt` |
| 테스트 | `mise run test` (race 검사기, 빌드한 바이너리의 종단 테스트 포함) |
| CI와 같은 검사 | `mise run check` |
| 빌드 | `mise run build` → `dist/orai` |
| 이 checkout 진단 | `mise run doctor` |

외부 의존성을 늘리지 않는다(현재 `BurntSushi/toml`, `fsnotify`). 테스트에서 바꿔 끼워야 하는 동작은 패키지 수준 함수 변수로 노출한다.

템플릿(`internal/scaffold/templates/`)을 바꾸면 바이너리에 내장되므로, `go run ./cmd/orai setup --no-tools`로 이 저장소의 생성 파일도 갱신한다(`TestThisRepositoryIsSetupCurrent`가 누락을 잡는다).

## 탐색 도구

| 작업 | 도구 |
|---|---|
| 코드 구조·심볼·호출 관계 | 저장소 루트에 `.codegraph/`가 있으면 CodeGraph(`codegraph explore "<질문>"`, `codegraph node <심볼>`)를 먼저 쓴다. 색인이 없거나 실패하면 `rg`와 원문 읽기로 이어간다 |
| 정확한 문자열·파일 | `rg -n '패턴' cmd internal docs`, `rg --files` |
| 설계·운영 문서 탐색 | 역할 세션에서는 MCP `wiki-orai`, 밖에서는 `orai doctor`로 서버 상태를 확인한 뒤 원문을 읽는다. 이 저장소의 문서는 적어서 원문을 바로 읽어도 된다 |
| 외부 도구 capability | 설치된 버전의 `--help`와 소스를 기준으로 한다. upstream main 문서와 설치 버전이 같다고 가정하지 않는다 |

## 문서 책임

| 문서 | 소유 내용 |
|---|---|
| `README.md` | 설명, 빠른 시작, 상태 요약, 문서 인덱스 |
| `docs/architecture.md` | 책임·경계, 식별·상태·메일함·데이터 흐름, 진단 모델, 패키지 지도 |
| `docs/operations.md` | 설치·실행·중지·진단·복구·검증 절차 |
| `docs/compatibility.md` | 플랫폼, 도구 버전·capability·설치 출처, 라이선스 |
| `docs/history.md` | 추출 원천 SHA, 알려진 장애, 변환 내역, 검증 기록 |
| `docs/release.md` | 배포 단계, 버전 정책, 릴리스 절차 |

검증 결과를 보고할 때는 fake·fixture 통과, 실제 provider 동작, 의미 검색 품질을 구분한다. 실행하지 않은 범위는 미검증으로 적는다.

<!-- orai:begin (managed by `orai setup`; edit outside this block) -->
## Orai

이 프로젝트의 역할 세션은 Orai로 실행한다. 역할과 연동 설정은 `orai.toml`에 있다.

- 메시지 수신·회신·전송(`orai msg`): `.agents/skills/orai/SKILL.md`
- 세션 상태와 진단: `orai status`, `orai doctor` (의미 검색까지 확인하려면 `--deep`)
- 문서 검색: 역할 세션에 연결된 `wiki-<프로젝트>` MCP 서버를 쓴다. 서버 상태는 `orai doctor`, 복구는 `orai wiki recover`로 한다.
- 코드 구조: `.codegraph/`가 있으면 CodeGraph를 먼저 쓴다.
- 두 도구 모두 실패하거나 결과가 비어도 대상이 없다는 근거로 삼지 않는다. `rg`와 원문 읽기로 이어간다.
<!-- orai:end -->
