# Moonflux

Учебный блог на Go и PostgreSQL с трёхслойной архитектурой.

## Текущий этап

- YAML-конфигурация с валидацией;
- структурированное логирование через `log/slog`;
- пул PostgreSQL через `pgx/v5/pgxpool`;
- собственный транзакционный исполнитель SQL-миграций;
- UUID v7 на `crypto/rand`;
- начальные таблицы `users` и `auth_sessions` с логическим удалением;
- HTTP-сервер с graceful shutdown, `GET /health`, `GET /ready` и access middleware.

## Запуск

1. Установите Go 1.25+ и PostgreSQL.
2. Создайте БД и пользователя PostgreSQL.
3. Скопируйте `config/config.example.yml` в `config/config.yml` и отредактируйте DSN.
4. Не коммитьте `config/config.yml`: файл уже включён в `.gitignore`.
5. Выполните:

```sh
go mod tidy
go run ./cmd/moonflux -config config/config.yml migrate up
go run ./cmd/moonflux -config config/config.yml
```

Для Docker указывайте `app.http_address: "0.0.0.0:8080"`, а порт публикуйте, например, как `10000:8080`. После применения миграций проверьте `curl -i http://localhost:10000/health` и `curl -i http://localhost:10000/ready`. `/health` проверяет, что HTTP-процесс отвечает; `/ready` дополнительно проверяет соединение с PostgreSQL. Корневой маршрут `/` пока не реализован и может вернуть 404.

## Миграции

Файлы именуются `NNNNNN_description.up.sql` и `NNNNNN_description.down.sql`. Команда `migrate up` применяет только новые `up`-миграции в транзакциях. `down`-файлы пока хранятся как документация для ручного анализа; автоматический откат не реализован, чтобы не провоцировать случайную потерю данных. Для прикладных данных физическое удаление запрещено политикой проекта.

## Логирование и аудит

- HTTP access log предназначен для всех запросов, включая ответы 4xx/5xx; поля включают `request_id`, `client_ip`, `remote_ip`, `ip_source`, путь без query-параметров, статус, длительность и User-Agent.
- Заголовки `X-Forwarded-For` доверяются только тогда, когда непосредственный `RemoteAddr` входит в `http.trusted_proxies`. В остальных случаях они игнорируются.
- `audit_logs` хранит значимые события безопасности отдельно от технических логов. Репозиторий аудита предоставляет только добавление событий.
- Срок хранения пока не включён (`retention_enabled: false`), пока не согласован конкретный период. Автоматическая очистка не реализована.
- Не логируйте пароли, их хеши, JWT, refresh-токены, `Authorization`, cookies и тела запросов.

## HTTP-проверки

- `GET /health` — liveness, отвечает `200` и `{"status":"ok"}`.
- `GET /ready` — readiness, проверяет PostgreSQL и отвечает `200` или `503`.
- Сервер обрабатывает `SIGINT`/`SIGTERM` и пытается завершить активные запросы за `app.shutdown_timeout`.
- Для Docker `app.http_address` должен быть `0.0.0.0:8080`, а не `127.0.0.1:8080`.
