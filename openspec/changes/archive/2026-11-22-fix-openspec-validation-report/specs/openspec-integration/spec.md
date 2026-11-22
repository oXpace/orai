## MODIFIED Requirements

### Requirement: 새 프로젝트는 OpenSpec 연동을 선언한다
`orai setup`이 새로 만드는 `orai.toml`은 `[integrations.openspec]`과 shelf 묶음 `openspec/specs`를 포함해야 한다(SHALL). 기존 `orai.toml`은 고치지 않는다.

#### Scenario: 빈 폴더에서 setup
- **WHEN** 빈 프로젝트에서 `orai setup --no-tools`를 실행한다
- **THEN** `orai.toml`에 `[integrations.openspec]`과 `specs = "openspec/specs"` 묶음이 있다

#### Scenario: 표가 없는 기존 프로젝트
- **GIVEN** `[integrations.openspec]`이 없는 `orai.toml`
- **WHEN** `orai setup`을 실행한다
- **THEN** `openspec/` 파일을 만들지 않고 doctor의 `openspec`은 not-configured다

### Requirement: deep 진단은 스펙을 검증한다
`orai doctor --deep`은 `openspec validate --specs` 결과를 `openspec.specs`로 보고해야 한다(SHALL). 검증 명령의 종료 코드가 0이 아니어도 보고서를 읽어 실패한 스펙 이름을 보여 주고, 결과를 읽을 수 없거나 실패한 스펙 없이 명령이 실패하면 healthy로 보고하지 않아야 한다(MUST).

#### Scenario: 스펙 하나가 잘못됐다
- **GIVEN** Purpose가 없는 스펙
- **WHEN** 검증 명령이 보고서를 출력하고 종료 코드 1로 끝난다
- **THEN** `openspec.specs`가 degraded이고 그 스펙 이름이 나온다
