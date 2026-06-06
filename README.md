# go-crud-api

Gin과 pgx(PostgreSQL)를 사용해 구현한 User CRUD REST API 예제입니다.
모든 응답에 처리 시간을 담는 `X-Response-Time-Ms` 헤더를 추가하는 미들웨어와
swaggo 기반 Swagger UI 문서를 포함합니다.

## 기술 스택

| 구분 | 사용 기술 | 버전 |
| --- | --- | --- |
| 언어 | Go | 1.26.1 |
| 웹 프레임워크 | [gin-gonic/gin](https://github.com/gin-gonic/gin) | v1.12.0 |
| DB 드라이버/풀 | [jackc/pgx/v5](https://github.com/jackc/pgx) (`pgxpool`) | v5.9.2 |
| UUID | [google/uuid](https://github.com/google/uuid) | v1.6.0 |
| API 문서 | [swaggo/swag](https://github.com/swaggo/swag), [gin-swagger](https://github.com/swaggo/gin-swagger), [swaggo/files](https://github.com/swaggo/files) | swag v1.16.6 / gin-swagger v1.6.1 / files v1.0.1 |
| 데이터베이스 | PostgreSQL | 미확인 (코드상 버전 제약 없음) |

## 디렉터리 구조

```
.
├── main.go            # 엔트리포인트: 라우팅, 핸들러, UserStore(데이터 접근), 미들웨어가 모두 한 파일에 정의됨
├── docs/              # swag가 생성한 Swagger 문서
│   ├── docs.go
│   ├── swagger.json
│   └── swagger.yaml
├── go.mod
├── go.sum
└── .gitignore
```

> 현재 단일 파일(`main.go`) 구성이며, 별도 패키지/레이어 분리는 되어 있지 않습니다.

## 주요 구성 요소

- **`User`**: `id`(UUID 문자열), `name`, `email` 필드를 가진 도메인 모델.
- **`UserStore`**: `pgxpool.Pool`을 감싼 데이터 접근 계층. 생성 시 DB에 연결(`Ping`)하고
  `users` 테이블을 자동 생성(`CREATE TABLE IF NOT EXISTS`)합니다.
  스키마는 `id TEXT PRIMARY KEY`, `name TEXT NOT NULL`, `email TEXT NOT NULL UNIQUE`.
- **`Handler`**: Gin 핸들러 모음. 입력 검증 실패 시 `400`, 레코드 없음(`pgx.ErrNoRows`) 시 `404`,
  그 외 오류 시 `500`을 반환합니다.
- **`ResponseTime()` 미들웨어**: 응답 헤더에 `X-Response-Time-Ms`(처리 시간, ms 단위)를 추가합니다.
- **UUID 생성**: 유저 생성 시 `uuid.NewString()`으로 ID를 발급합니다.

## API 엔드포인트

Base path: `/api/v1`

| 메서드 | 경로 | 설명 | 성공 응답 |
| --- | --- | --- | --- |
| POST | `/api/v1/users` | 유저 생성 | `201` + `User` |
| GET | `/api/v1/users` | 유저 목록 (이름순 정렬) | `200` + `User[]` |
| GET | `/api/v1/users/:id` | 유저 단건 조회 | `200` + `User` |
| PUT | `/api/v1/users/:id` | 유저 수정 | `200` + `User` |
| DELETE | `/api/v1/users/:id` | 유저 삭제 | `204` (본문 없음) |

모든 응답에는 `X-Response-Time-Ms` 헤더가 포함됩니다.

### 요청 본문 (POST / PUT)

```json
{
  "name": "정선우",
  "email": "user@example.com"
}
```

`name`은 필수, `email`은 필수이며 이메일 형식 검증(`binding:"required,email"`)을 거칩니다.

### Swagger UI

서버 실행 후 브라우저에서 접속합니다.

```
http://localhost:8080/swagger/index.html
```

(Swagger 메타: title `User CRUD API`, version `1.0`, host `localhost:8080`, basePath `/api/v1`)

## 실행 방법

### 1. PostgreSQL 준비

기본 접속 정보(DSN)는 다음과 같으며, `DATABASE_URL` 환경 변수로 덮어쓸 수 있습니다.

```
postgres://postgres@localhost:5432/postgres?sslmode=disable
```

```bash
# 예: 커스텀 DB 지정
export DATABASE_URL="postgres://user:password@localhost:5432/mydb?sslmode=disable"
```

> 앱 시작 시 `users` 테이블이 없으면 자동 생성되므로 별도 마이그레이션은 필요하지 않습니다.

### 2. 서버 실행

```bash
go run .
```

서버는 `:8080` 포트에서 동작합니다. DB 연결에 실패하면 기동 시점에 종료됩니다.

### 3. 빌드

```bash
go build -o go-crud-api .
./go-crud-api
```

> Swagger 문서(`docs/`)는 이미 커밋되어 있습니다. 핸들러 주석을 변경한 뒤 재생성하려면
> `swag init`을 사용합니다. (swag CLI 별도 설치 필요)

## 미확인 사항

- Dockerfile / docker-compose, Makefile, CI 설정 등 부가 인프라 파일은 저장소에 없습니다.
- 테스트 코드(`*_test.go`)는 없습니다.
- PostgreSQL의 권장/검증 버전은 코드에 명시되어 있지 않습니다.
