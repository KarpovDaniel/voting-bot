# Этап сборки
FROM golang:1.23 AS builder
USER root

# Установка зависимостей
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libssl-dev \
    pkg-config \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Настройка рабочей директории
WORKDIR /app
COPY . .

# Сборка приложения
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags '-w -s' -o voting-bot

# Этап тестирования
FROM builder AS test
CMD ["go", "test", "-v", "./..."]

# Финальный образ
FROM debian:bullseye-slim
USER root
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libssl3 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/voting-bot /usr/local/bin/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 1001:1001
CMD ["voting-bot"]