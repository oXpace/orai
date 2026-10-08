# 문서

이 폴더에는 설계, 운영, 결정 기록처럼 오래 유지할 Markdown 문서를 둔다. 이 폴더는 shelf(프로젝트 문서 검색)에 등록돼 있어, 역할 세션이 `shelf` MCP 서버로 이 문서를 찾는다. 이 파일은 `orai setup`이 만든 시작 페이지이므로 자유롭게 고쳐 쓴다.

## shelf(문서 검색) 쓰는 법

| 할 일 | 명령 |
|---|---|
| 처음 한 번 색인 만들기 | `orai shelf init` (`orai setup`이 대신 실행한다) |
| 문서를 고친 뒤 색인 갱신 | `orai shelf sync` |
| 재부팅 뒤 서버 다시 켜기 | `orai shelf recover` |
| 상태 확인 | `orai doctor`, 실제 검색까지 확인하려면 `orai doctor --deep` |

- 검색 엔진은 QMD다. 없으면 `npm install -g @tobilu/qmd`로 설치한다(Node 22 이상).
- 다른 폴더도 검색하거나 폴더를 나눠 등록하려면 `orai.toml`의 `[integrations.shelf]`에서 `collections`를 고친다.
- 설정 항목과 문제 해결은 `orai shelf --help`에 있다.
