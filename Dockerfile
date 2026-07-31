# Образ, который собирает Railway (см. railway.toml: builder = "dockerfile").

# --- Сборка фронтенда ---
FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
# npm ci, а не install: ставим ровно то, что в lock-файле. Иначе прод мог бы
# уехать на версии, которую никто не проверял локально.
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Сборка Go-бинарника ---
FROM golang:1.25-alpine AS api
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# Миграции вшиваются в бинарник через embed.FS (migrations/migrations.go):
# на сборке каталог нужен, в рантайме — уже нет.
COPY migrations/ ./migrations/
RUN CGO_ENABLED=0 go build -o /cramflow ./cmd

# --- Рантайм ---
FROM alpine:3.20
# postgresql16-client — ради pg_dump: на нём держатся ночные бэкапы (BACKUP_CRON)
# и ручной /backup. Мажор клиента должен совпадать с мажором сервера Postgres —
# pg_dump отказывается снимать дамп с более новой базы. Railway Postgres сейчас 16;
# при переезде на 17 нужно поднять и этот пакет, и версию базы вместе.
RUN apk add --no-cache ca-certificates tzdata postgresql16-client
# Сборка падает сразу, если pg_dump не приехал: иначе поломка всплыла бы
# ночью, первым несделанным бэкапом.
RUN pg_dump --version

WORKDIR /app
COPY --from=api /cramflow ./cramflow
COPY --from=web /app/web/dist ./web/dist

# Каталоги под фото и дампы: код создаёт их сам, но с ними образ сразу
# соответствует переменным ниже.
RUN mkdir -p /app/uploads /app/backups

# Фото товаров: подключите Railway Volume в /app/uploads, чтобы файлы переживали
# деплой. Без Volume ставьте UPLOAD_STORE=db — иначе фото исчезнут при рестарте.
ENV UPLOAD_DIR=/app/uploads
ENV WEB_DIST=/app/web/dist
ENV BACKUP_DIR=/app/backups
# PORT Railway подставляет сам; 8080 — значение по умолчанию в коде.
EXPOSE 8080
CMD ["./cramflow"]
