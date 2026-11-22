# Tasks

## 1. 테스트 먼저

- [x] 1.1 `internal/shelf/builtin_test.go`와 `TestSearchDefaultsToTheBuiltInEngine`의 실패를 확인한다

## 2. 구현

- [x] 2.1 엔진 선택과 각 진입점(검색, setup, doctor, MCP, 자동 sync, 수명주기)을 바꾸고 테스트 통과를 확인한다
- [x] 2.2 템플릿, 도움말, 생성 파일을 고치고 `TestThisRepositoryIsSetupCurrent` 통과를 확인한다

## 3. 검증

- [x] 3.1 새 프로젝트: QMD가 PATH에 있어도 서버·색인 없이 setup healthy, search 종료 코드 0
- [x] 3.2 0.6.0 바이너리로 만든 QMD 프로젝트를 올려 의미 검색 유지, 색인 없는 0.6.0 프로젝트는 내장 검색
- [x] 3.3 `engine = "qmd"`로 켜서 실제 QMD init과 검색
- [x] 3.4 내장 검색의 품질(이 저장소 smoke)과 큰 묶음 속도를 재고 문서에 적는다
