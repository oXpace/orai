# Tasks

## 1. schema 1 거부

- [x] 1.1 `TestSchemaOneIsRefusedWithTheEdit`, `TestProjectFromZeroThreeIsToldWhatToEdit`를 먼저 바꾸고 실패를 확인한다
- [x] 1.2 `config.validate`가 schema 1을 두 줄 수정 문구로 거부하고 doctor의 schema 1 degraded 분기를 지운다. 위 테스트와 `TestLocalFileErrorsNameTheFile` 통과로 확인한다

## 2. 이전 경로 제거

- [x] 2.1 `.orai/wiki` 이전, 예전 명령 이름 안내 표(`TestUsageErrorsExitTwo`에서 해당 행 제거), `pypi:` 고정 인식을 지운다. `mise run check` 통과로 확인한다
- [x] 2.2 추적된 `__pycache__` 58개를 지운다. `git ls-files '*.pyc'`가 비어 있음으로 확인한다
- [x] 2.3 운영 안내·아키텍처·이력·README 상태 표를 고친다. `TestThisRepositoryIsSetupCurrent` 통과와 문서 링크로 확인한다

## 3. 병합

- [x] 3.1 PR #1 리뷰 반영 후 병합하고, trunk에서 `mise run check` 통과를 확인한다(2026-11-22 trunk 4d2886c, gofmt·vet·`go test -race` 통과, Linux)

## Workflow follow-up

- 병합 뒤 `openspec archive drop-legacy-upgrade-paths`로 `project-config` 스펙을 만든다.
- 0.7.0 릴리스 노트에 BREAKING 두 항목을 적는다.
