# 배포 계획

이 문서는 Orai의 배포 경로, 버전 정책, 릴리스 점검 항목을 소유한다. 아직 외부에 배포한 버전은 없다.

## 배포 단위와 설치 방식

Orai는 표준 라이브러리만 쓰는 순수 Python 패키지(`orai`, console script `orai`)다. 외부 도구(AMQ, Codex, Claude Code, QMD, CodeGraph)는 번들하지 않고 사용자 설치를 진단한다.

설치는 **프로젝트 단위**다. 각 프로젝트의 `mise.toml`에 `"pypi:oXpace/orai" = "<버전>"`처럼 고정하고, mise의 `pypi` backend가 uv로 설치한다. 프로젝트마다 버전이 독립적이며, 저장소를 받은 사람은 `mise install`만 하면 같은 버전을 쓴다. 전역 설치(`uv tool install`)는 권장하지 않는다.

## 단계

| 단계 | 경로 | 진입 조건 |
|---|---|---|
| 0. 로컬 | Orai checkout에서 `mise run setup` | 지금 |
| 1. GitHub 게시 | 공개 저장소 `oXpace/orai` push, GitHub Release `X.Y.Z` 생성 | CI 통과 |
| 2. 프로젝트 설치 검증 | 빈 폴더에서 `mise use pypi:oXpace/orai@X.Y.Z` → `orai setup` | 1단계 직후. mise `pypi` backend가 GitHub Release/태그를 어떻게 해석하는지 이때 확인한다(로컬 원천으로는 검증 불가) |
| 3. 파일럿 | 격리된 파일럿 저장소에서 Codex↔Claude 실제 검증 ([운영 안내](operations.md#실제-파일럿-계정호스트-준비-후)) | 2단계 설치본으로 수행 |
| 4. 소비 프로젝트 이전 | Pockets: `scripts/orai`와 `~/.local/bin/orai` 전역 링크 → 프로젝트 고정 `orai`, `.agents/orai.json` → `orai.toml` | 파일럿 통과. 별도 작업으로 진행하며 이 저장소는 Pockets를 수정하지 않음 |
| 5. PyPI | `mise use pypi:orai@X.Y.Z` | 외부 사용자 요구가 생겼을 때. `orai` 이름은 2026-09-25 기준 PyPI에 미등록(조회 결과 404) |

PyPI 게시는 GitHub Actions의 Trusted Publishing(OIDC)을 쓰는 방식을 권장한다. 장기 토큰을 저장소 secret에 두지 않아도 된다. PyPI로 옮기면 `scaffold.INSTALL_SPEC`과 문서의 설치 명령을 함께 바꾼다.

## 버전 정책

- `0.x` 동안은 minor 버전에서 호환성이 깨질 수 있다. 깨지는 변경은 릴리스 노트에 명시한다.
- 호환 계약에 포함되는 것: CLI 명령과 옵션, 종료 코드, `doctor`·`status`·`--dry-run`의 JSON 필드, `orai.toml` schema, 로컬 상태 구조, 알림 문구.
- `orai.toml` schema가 바뀌면 `schema` 번호를 올리고, 이전 schema를 명시적으로 import하는 경로를 제공한다.

## 릴리스 점검

1. `mise run check` 통과 (lint, test, package 설치 테스트). CI의 macOS·Linux와 Python 3.11·3.14 매트릭스 통과.
2. [호환성](compatibility.md) 표를 실제 확인 버전으로 갱신.
3. `pyproject.toml` 버전 갱신 → `uv lock` → GitHub Release `X.Y.Z` 생성(`gh release create X.Y.Z`). 태그에 `v`를 붙이지 않는다. mise는 Release 태그 이름을 그대로 버전으로 쓰므로(`v1.0.0` → `@v1.0.0`) 설치 명령을 `@X.Y.Z`로 맞추기 위해서다. mise는 태그가 아니라 Release 목록을 읽으므로 Release가 있어야 한다.
4. `uv build`로 만든 wheel을 새 venv에 설치해 `orai --version`, `orai setup` 두 번 실행(두 번째는 무변경), `orai doctor` 확인. `test:package`가 이 과정을 자동으로 수행한다. 게시 후에는 빈 폴더에서 `mise use` → `orai setup`을 실제로 수행한다.
5. 릴리스 노트: 변경, 호환성 영향, 검증 범위(실제 provider 파일럿 여부 포함).

## 결정된 항목 (2026-09-25)

- 라이선스: MIT, 저작권자 oXpace. 의존 도구 검토는 [호환성](compatibility.md#라이선스)에 있다.
- 저장소: `github.com/oXpace/orai`, 공개.
- 설치 원천: GitHub Release(`pypi:oXpace/orai@X.Y.Z`). mise는 Release 목록에서 버전을 읽고 `uv tool install git+https://github.com/oXpace/orai.git@X.Y.Z`로 설치한다(`psf/black`으로 동작 확인).
- PyPI 이름 `orai` 선점은 필요할 때 결정한다.
