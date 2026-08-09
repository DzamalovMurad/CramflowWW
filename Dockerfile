# --- Сборка фронтенда ---
FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
# npm ci — строго по lock-файлу: сборка воспроизводима и не подтянет
# неожиданную версию зависимости между деплоями.
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
COPY migrations/ ./migrations/
# Миграции вшиты в бинарник через //go:embed — в рантайм их копировать не нужно.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /flowix ./cmd

# --- Рантайм ---
FROM alpine:3.21
# postgresql16-client нужен ради pg_dump: его запускает фоновое задание бэкапа.
# tzdata — часовой пояс магазина (Europe/Moscow), иначе даты доставки съезжают.
# ca-certificates — HTTPS к Telegram API.
# su-exec — понижение привилегий в entrypoint: том Railway приходит от root,
# и владельца нужно поправить до запуска приложения.
RUN apk add --no-cache ca-certificates tzdata postgresql16-client su-exec \
    && adduser -D -u 10001 flowix

WORKDIR /app
COPY --from=api /flowix ./flowix
COPY --from=web /app/web/dist ./web/dist
# Каталог на случай UPLOAD_STORE=local; по умолчанию фото хранятся в БД.
RUN mkdir -p /app/uploads && chown -R flowix:flowix /app
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
# USER здесь не ставим: entrypoint стартует от root, чинит владельца
# смонтированного тома и сам переключается на flowix через su-exec.

ENV WEB_DIST=/app/web/dist \
    UPLOAD_DIR=/app/uploads \
    TZ=Europe/Moscow
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["./flowix"]
