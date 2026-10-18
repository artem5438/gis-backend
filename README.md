# GIS Backend

Бэкенд геоинформационной системы оценки инвестиционного потенциала Норильска.
Лабораторная 1: каркас сервиса, хранилища и проверка живости.

- Frontend: https://github.com/artem5438/gis-frontend
- Backend: https://github.com/artem5438/gis-backend

## Стек

- [Go](https://go.dev/doc/)
- [chi](https://github.com/go-chi/chi)
- [PostgreSQL 16](https://www.postgresql.org/docs/16/)
- [Redis](https://redis.io/docs/)
- [MinIO](https://min.io/docs/minio/container/index.html)
- [Docker Compose](https://docs.docker.com/compose/)

База, Redis и MinIO в этой лабораторной поднимаются, но API к ним ещё не обращается. Образ PostgreSQL обычный: расширение PostGIS подключим вместе с геометрией площадок.

## Структура

```
config/            чтение переменных окружения и файла .env
scripts/           инициализация бакета MinIO
main.go            маршруты HTTP
docker-compose.yml PostgreSQL, Redis, MinIO, Adminer, backend, frontend
```

Имена файлов Go — в нижнем регистре, пакеты совпадают с каталогом. Компоненты фронтенда именуются в PascalCase.

## Переменные окружения

Скопируйте пример и поправьте при необходимости:

```bash
cp .env.example .env
```

| Переменная | Назначение | По умолчанию |
|---|---|---|
| `HTTP_ADDR` | адрес HTTP-сервера | `:8080` |
| `CORS_ORIGIN` | разрешённые origin через запятую | `http://localhost:5173,http://localhost` |
| `DB_HOST` | хост PostgreSQL | `localhost` |
| `DB_PORT` | порт PostgreSQL | `5432` |
| `DB_USER` | пользователь БД | `admin` |
| `DB_PASSWORD` | пароль БД | `secret` |
| `DB_NAME` | имя базы | `norilsk_gis` |
| `REDIS_ADDR` | адрес Redis | `localhost:6379` |
| `MINIO_ENDPOINT` | адрес MinIO | `localhost:9000` |
| `MINIO_ACCESS_KEY` | ключ MinIO | `minioadmin` |
| `MINIO_SECRET_KEY` | секрет MinIO | `minioadmin123` |
| `MINIO_BUCKET` | бакет для фото площадок | `norilsk-media` |
| `MINIO_USE_SSL` | `true` или `false` | `false` |

Файл `.env` не коммитится. Уже заданные переменные окружения он не перезаписывает, поэтому значения из Compose остаются в силе внутри контейнера.

## Запуск

Весь стенд, включая собранный фронтенд:

```bash
docker compose up --build
```

Проверка:

```bash
curl -s http://localhost:8080/api/health
```

Ответ: `{"status":"ok"}`. Сайт открывается на http://localhost. Adminer: http://localhost:8082. Консоль MinIO: http://localhost:9001.

Только API на хосте, пока хранилища работают в Docker:

```bash
docker compose up -d postgres redis minio minio-init adminer
go run .
```

## Линтер и pre-commit

Нужны [golangci-lint](https://golangci-lint.run/) и [pre-commit](https://pre-commit.com/).

```bash
golangci-lint run ./...
pre-commit install
pre-commit run --all-files
```

Коммит не проходит, если в индексе файл больше 500 КБ, остались маркеры конфликта или `gofmt` / `go vet` / golangci-lint находят ошибку.
