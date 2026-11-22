# 문서

이 폴더에는 설계, 운영, 결정 기록처럼 오래 유지할 Markdown 문서를 둔다. 이 폴더는 shelf(프로젝트 문서 검색)에 등록돼 있어, `orai search`로 이 문서를 찾는다. 이 파일은 `orai setup`이 만든 시작 페이지이므로 자유롭게 고쳐 쓴다.

## shelf(문서 검색) 쓰는 법

| 할 일 | 명령 |
|---|---|
| 문서 찾기 | `orai search '<질문>' --in docs` |
| 상태 확인 | `orai doctor`, 실제 검색까지 확인하려면 `orai doctor --deep` |

- 기본은 내장 키워드 검색이다. 설치, 서버, 색인이 필요 없고 고친 문서가 바로 검색된다.
- 의미 검색이 필요하면 QMD를 켠다: `npm install -g @tobilu/qmd`(Node 22 이상), `orai.toml`의 `[integrations.shelf]`에 `engine = "qmd"`, `orai shelf init`.
- 다른 폴더도 검색하거나 폴더를 나눠 등록하려면 `orai.toml`의 `[integrations.shelf]`에서 `collections`를 고친다.
- 설정 항목과 문제 해결은 `orai shelf --help`에 있다.
