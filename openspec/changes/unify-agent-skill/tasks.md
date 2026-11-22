# Tasks

## 1. 스킬 없는 init

- [x] 1.1 init 인자, 준비 판정, 기존 스킬 보존, 진단의 실패 테스트를 쓰고 실패를 확인한다
- [x] 1.2 `internal/openspec`과 setup 도움말을 고치고 `go test ./internal/openspec ./internal/cli` 통과를 확인한다
- [x] 1.3 architecture, operations, compatibility, README의 init 설명을 고치고 `rg 'openspec-propose|--scope project' docs README.md`로 남은 옛 설명이 없는지 확인한다

## 2. 통합 스킬

- [x] 2.1 description, 단계 절, 보관 순서, 라이선스 고지의 실패 테스트를 쓰고 실패를 확인한다
- [x] 2.2 스킬 템플릿과 AGENTS 블록을 고치고 `go test ./internal/scaffold` 통과를 확인한다
- [x] 2.3 history에 원천 버전을, README와 compatibility에 재배포 범위를 적고 문구가 고지 파일과 맞는지 확인한다

## 3. 이 저장소에 적용

- [x] 3.1 `orai.toml`에 OpenSpec 연동과 specs 묶음을 넣고 `go run ./cmd/orai setup --no-tools` 뒤 `TestThisRepositoryIsSetupCurrent` 통과를 확인한다
- [x] 3.2 `mise run check` 통과를 확인한다 (2026-11-22, `amq`는 PATH에서 제외)

## 4. 실제 동작

- [x] 4.1 격리 HOME의 새 프로젝트에서 실제 OpenSpec 2.1.1로 setup부터 보관까지 돌려 스킬이 하나만 남는지 확인한다

## Workflow follow-up

- Codex와 Claude 새 세션이 propose 요청에 Orai 스킬을 고르는지는 `verify-real-hosts` 1.5에서 확인한다.
- PR 자체 리뷰 뒤 change를 보관한다.
