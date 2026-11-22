# Tasks

## 1. 연동 선언과 설정

- [x] 1.1 `TestOpenSpecTableKeepsTelemetryOffUnlessTurnedOn`, `TestNotDeclaredIsNotConfigured`, `TestNoOpenSpecFilesWithoutTheTable` 통과로 확인한다
- [x] 1.2 새 `orai.toml` 템플릿과 시작 config.yaml: `TestNewProjectDeclaresOpenSpecWithTestFirstStarterRules`, `TestOpenSpecConfigIsNeverRewritten`, `TestOpenSpecStarterUsesOpenSpecKeys` 통과로 확인한다

## 2. setup과 진단

- [x] 2.1 프로젝트 범위 init: `TestSetupInitializesAtProjectScopeOnce`, `TestMissingBinaryBlocksDoctorAndSetupOnlySkips`, `TestOldOpenSpecIsUnsupportedAndNotRun`, `TestNotInitializedIsNotReady` 통과로 확인한다(fake openspec)
- [x] 2.2 deep 검증: `TestDeepValidationNamesFailingSpecs`, `TestDeepValidationOfValidSpecsIsHealthy`, `TestUnreadableValidationOutputIsNotHealthy` 통과로 확인한다
- [x] 2.3 텔레메트리: `TestChildEnvKeepsOpenSpecTelemetryOffUnlessTheProjectTurnsItOn`, `TestTelemetryStaysWhenTheProjectTurnsItOn` 통과로 확인한다
- [x] 2.4 실제 OpenSpec 2.1.1로 init, config.yaml 보존, rules 주입을 확인한다(Linux 클라우드, `docs/history.md`)

## 3. 작업 안내

- [x] 3.1 `skill-workflow.md` 템플릿, AGENTS 블록 한 줄, manager·engineer·role 템플릿을 바꾼다. `TestThisRepositoryIsSetupCurrent` 통과로 확인한다

## 4. 전달

- [x] 4.1 전달 방식을 정한다: PR #4로 올림
- [x] 4.2 PR #1과의 충돌을 풀어 통합 PR #8로 병합하고 trunk에서 검사 통과를 확인한다(2026-11-22 trunk 4d2886c)
- [x] 4.0 4.3은 보관하면서 `verify-real-hosts`(1.4)로 옮긴다
- [ ] 4.3 사용자 Mac에서 `orai setup`으로 실제 init과 doctor `openspec` 항목을 확인한다

## Workflow follow-up

- 병합 뒤 이 저장소의 `orai.toml`에도 `[integrations.openspec]`과 `specs` 묶음을 넣을지 정한다.
