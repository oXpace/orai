# 문서

이 폴더는 프로젝트 wiki의 원천이다. 설계, 운영, 결정 기록처럼 오래 유지할 Markdown 문서를 둔다. 역할 세션은 `wiki-<프로젝트>` MCP 서버로 이 문서를 검색한다. 이 파일은 `orai setup`이 만든 시작 페이지이므로 자유롭게 고쳐 쓴다.

## wiki 쓰는 법

| 할 일 | 명령 |
|---|---|
| 처음 한 번 색인 만들기 | `orai wiki init` (`orai setup`이 대신 실행한다) |
| 문서를 고친 뒤 색인 갱신 | `orai wiki stop && orai wiki refresh` |
| 재부팅 뒤 서버 다시 켜기 | `orai wiki recover` |
| 상태 확인 | `orai doctor`, 실제 검색까지 확인하려면 `orai doctor --deep` |

- 검색 엔진은 QMD다. 없으면 `npm install -g @tobilu/qmd`로 설치한다(Node 22 이상).
- 다른 폴더도 검색하려면 `orai.toml`의 `[integrations.wiki]`에서 `collections`에 추가한다.
- 설정 항목과 문제 해결은 `orai wiki --help`에 있다.
