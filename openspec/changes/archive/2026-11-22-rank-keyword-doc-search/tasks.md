# Tasks

## 1. 절 단위 랭킹

- [x] 1.1 절 나누기와 낱말 형태: `TestParseMarkdownSplitsAtHeadingsOutsideCode`, `TestVariantsTrimHangulTailsAndSplitMixedWords` 통과로 확인한다
- [x] 1.2 IDF·제목 가중치·짧은 질문 조건: `TestSearchDocsRanksByRareWordsAndPointsAtTheSectionLine`, `TestSearchDocsWeighsTheTitleAndHeading`, `TestSearchDocsShortQueriesNeedEveryWord` 통과로 확인한다

## 2. 여러 표현과 약한 결과

- [x] 2.1 `TestSearchDocsFusesPhrasingsAndSaysWhenWeak`와 CLI의 `--also`·weak 검사(`internal/cli/harness_test.go`) 통과로 확인한다
- [x] 2.2 운영·아키텍처 문서와 스킬 템플릿을 고친다. `TestThisRepositoryIsSetupCurrent` 통과로 확인한다
- [x] 2.3 이 저장소 `docs/`에서 이전 바이너리와 비교한다(`docs/history.md` 2026-11-22 기록, smoke 질문의 첫 결과가 정답으로 바뀜)

## 3. 병합

- [x] 3.1 PR #7 병합 후 trunk에서 검사 통과를 확인한다(2026-11-22 trunk 4d2886c)

## Workflow follow-up

- 실제 QMD 서버에 여러 표현을 보내는 질의와 큰 묶음에서의 품질·속도는 `verify-real-hosts` 1.1에서 확인한다.
