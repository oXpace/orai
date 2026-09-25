# 배포 계획

이 문서는 Orai의 배포 경로, 버전 정책, 릴리스 점검 항목을 소유한다.

## 배포 단위와 설치 방식

Orai는 Go 단일 실행 파일(`orai`)이다. 외부 도구(Codex, Claude Code, QMD, CodeGraph)는 번들하지 않고 사용자 설치를 진단한다. 메일함은 Orai가 직접 관리하므로 AMQ가 필요 없다.

설치는 **프로젝트 단위**다. 각 프로젝트의 `mise.toml`에 `"github:oXpace/orai" = "<버전>"`으로 고정하면, mise github backend가 GitHub Release에 첨부된 해당 플랫폼 바이너리를 받는다. 프로젝트마다 버전이 독립적이며, 저장소를 받은 사람은 `mise install`만 하면 같은 버전을 쓴다.

| 버전 | 형태 | 설치 명령 |
|---|---|---|
| 0.1.0 | Python 패키지 (AMQ 필요) | `mise use pypi:oXpace/orai@0.1.0` |
| 0.2.0~ | Go 바이너리 (AMQ 불필요) | `mise use github:oXpace/orai@0.2.0` |

## 릴리스 절차

1. `mise run check` 통과: gofmt, go vet, race 검사기를 켠 전체 테스트(빌드한 바이너리의 종단 테스트 포함). CI의 macOS(실제 AMQ 교차 호환 포함)·Linux와 4개 타깃 크로스 빌드 통과.
2. [호환성](compatibility.md) 표를 실제 확인 버전으로 갱신.
3. GitHub Release를 태그 `X.Y.Z`로 만든다(`gh release create X.Y.Z --target trunk`). 태그에 `v`를 붙이지 않는다. mise는 Release 태그 이름을 그대로 버전으로 쓰므로(`v1.0.0` → `@v1.0.0`), 설치 명령을 `@X.Y.Z`로 맞추기 위해서다.
4. Release 게시가 `release` 워크플로를 실행한다. 워크플로는 `orai_X.Y.Z_{darwin,linux}_{arm64,amd64}.tar.gz`를 빌드해 첨부한다. 버전은 `-ldflags`로 바이너리에 들어간다.
5. 빈 폴더에서 `mise use github:oXpace/orai@X.Y.Z` → `orai --version` → `orai setup`을 실제로 수행한다.
6. 릴리스 노트: 변경, 호환성 영향, 검증 범위(실제 provider 파일럿 여부 포함).

## 단계

| 단계 | 내용 | 진입 조건 |
|---|---|---|
| 1. 공개 배포 | `github.com/oXpace/orai` 공개, Release | 완료 (0.1.0, 0.2.0) |
| 2. 파일럿 | 격리된 파일럿 저장소에서 Codex↔Claude 실제 검증 ([운영 안내](operations.md#실제-파일럿-계정호스트-준비-후)) | 계정 동의가 필요해 사용자가 실행 |
| 3. 소비 프로젝트 이전 | Pockets: `scripts/orai`와 `~/.local/bin/orai` 전역 링크 → 프로젝트 고정 `orai`, `.agents/orai.json` → `orai.toml`. 메일함 경로(`.agent-mail/orai`)는 같아서 기존 메일이 그대로 읽힌다 | 파일럿 통과. 별도 작업으로 진행하며 이 저장소는 Pockets를 수정하지 않음 |
| 4. 작업 목록 | 메일함 위에 역할 간 작업 배정·임대·의존성 | 설계 문서 합의 후 |

## 버전 정책

- `0.x` 동안은 minor 버전에서 호환성이 깨질 수 있다. 깨지는 변경은 릴리스 노트에 명시한다.
- 호환 계약에 포함되는 것: CLI 명령과 옵션, 종료 코드, `doctor`·`status`·`msg`·`--dry-run`의 JSON 필드, `orai.toml` schema, 로컬 상태 구조, 메일함 디스크 형식(AMQ schema 1 호환), 알림 문구.
- `orai.toml` schema가 바뀌면 `schema` 번호를 올리고, 이전 schema를 명시적으로 가져오는 경로를 제공한다.

## 결정된 항목 (2026-09-25)

- 라이선스: MIT, 저작권자 oXpace. 의존성 검토는 [호환성](compatibility.md#라이선스)에 있다.
- 저장소: `github.com/oXpace/orai`, 공개.
- 구현 언어: Go (0.2.0). 근거는 [이력](history.md#go-전환-020)에 있다.
- 설치 원천: GitHub Release 바이너리(`github:oXpace/orai@X.Y.Z`).
