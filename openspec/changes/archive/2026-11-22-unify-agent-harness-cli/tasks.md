# Tasks

## 1. 메시지 명령

- [x] 1.1 `TestShortMessageCommandsMatchMsg`를 먼저 쓰고 `orai inbox|send|reply`를 `msg`와 같은 함수로 연결한다. 테스트 통과로 확인한다
- [x] 1.2 본문 입력 형식과 대화형 터미널 거부: 기존 `TestInteractiveStdinWithoutBodyIsRejectedBeforeReading` 통과로 확인한다. 위치 인자 본문의 전용 테스트는 `close-test-gaps`로 넘긴다

## 2. 검색과 읽기

- [x] 2.1 메일 검색: `TestSearchReadsReceivedAndSentButOnlyCountsPending`, `TestShowRefusesAMessageNotReceivedAndChangesNothing` 통과로 확인한다
- [x] 2.2 키워드 매칭(`internal/lexical`): `TestTermsLowercaseDeduplicateAndStemKorean`, `TestScoreFindsTheBestLineAndTheStemMatches`, `TestShortQueriesNeedEveryTerm` 통과로 확인한다
- [x] 2.3 문서 검색과 대체 검색: `TestSearchUsesTheEngineAndMapsPathsToTheProject`, `TestSearchFallsBackToKeywordsWithTheReasonWhenNotBuilt`, `TestSearchReportsAServerThatIsDownAndDocumentsChangedSinceSync` 통과로 확인한다(fake 서버)
- [x] 2.4 `orai search`·`orai read` CLI: `TestSearchCoversTheRolesMailboxAndDocsWithoutReceiving`, `TestSearchOutsideARoleSkipsMailUnlessAsked`, `TestReadPrintsLineRangesAndStaysInsideTheProject` 통과로 확인한다
- [x] 2.5 깨진 메시지를 건너뛰는 메일 검색: `TestSearchSkipsUnreadableMessages` 통과로 확인한다(1832c98)

## 3. 문서 자동 sync

- [x] 3.1 `TestAutoSyncSyncsAfterADocumentChangesAndNotForOtherFiles`, `TestAutoSyncNeverBuildsAShelfThatWasNotBuilt` 통과로 확인한다
- [x] 3.2 세션 시작 때 따라잡기: 코드(`internal/shelf/autosync.go`) 검토로 확인. 전용 테스트는 `close-test-gaps`로 넘긴다

## 4. 코드 그래프 운영

- [x] 4.1 `TestFoldersAndFolderForPreferTheNearestGraph`, `TestMCPServerIsServedFromTheRolesOwnBuiltCheckout`, `TestSearchParsesSymbolsAndReportsPendingChanges` 통과로 확인한다(fake codegraph)
- [x] 4.2 AGENTS 블록·스킬·운영 문서를 다섯 동사 기준으로 바꾼다. `TestThisRepositoryIsSetupCurrent` 통과로 확인한다

## 5. 실제 환경 확인 (사용자 Mac)

실제 환경 확인(5.1~5.3)은 보관하면서 `verify-real-hosts`로 넘긴다.

- [x] 5.0 5.1~5.3을 `verify-real-hosts`(1.1~1.3)로 옮긴다
- [ ] 5.1 실제 QMD 서버 경유 `orai search` 결과와 상태 줄을 확인한다
- [ ] 5.2 역할 세션 중 문서를 고쳐 `.orai/shelf/autosync.log`에 sync가 남는지 확인한다
- [ ] 5.3 역할 세션에서 `codegraph` MCP 도구가 보이고 자기 worktree를 읽는지 확인한다. 전역 `codegraph` 등록과의 우선순위도 적는다
- [x] 5.4 PR #2(통합 #6) 병합 후 trunk에서 검사 통과를 확인한다(2026-11-22 trunk 4d2886c)

## Workflow follow-up

- 병합 순서(#1 → #3 → #2 → #7 → #4)대로 보관한다.
