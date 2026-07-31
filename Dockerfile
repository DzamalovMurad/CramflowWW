# --- Сборка фронтенда ---
FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
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
# postgresql16-client — ради pg_dump для ночных бэкапов (см. README).
RUN apk add --no-cache ca-certificates tzdata postgresql16-client
WORKDIR /app
COPY --from=api /cramflow ./cramflow
COPY --from=web /app/web/dist ./web/dist
# Фото товаров: подключите Railway Volume в /app/uploads, чтобы файлы переживали деплой
ENV UPLOAD_DIR=/app/uploads
ENV WEB_DIST=/app/web/dist
ENV BACKUP_DIR=/app/backups
EXPOSE 8080
CMD ["./cramflow"]
