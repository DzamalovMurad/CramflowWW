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
RUN CGO_ENABLED=0 go build -o /cramflow ./cmd

# --- Рантайм ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=api /cramflow ./cramflow
COPY --from=web /app/web/dist ./web/dist
COPY migrations/ ./migrations/
# Фото товаров: подключите Railway Volume в /app/uploads, чтобы файлы переживали деплой
ENV UPLOAD_DIR=/app/uploads
ENV WEB_DIST=/app/web/dist
EXPOSE 8080
CMD ["./cramflow"]
