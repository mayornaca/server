# Testing — Cómo correr unit, e2e y smoke

Documento vivo (cloud-gesvial.22.1.2). Todos los comandos se ejecutan desde `gesvial-sos-monitor-server/` salvo que se indique lo contrario.

## Resumen

| Tipo | Comando | Tiempo aprox. | Cuándo correr |
|---|---|---|---|
| Lint + fmt | `make fmt && make lint` | <30s | Antes de cada commit (idealmente en hook). |
| Unit tests | `make test` | ~30-60s | Antes de cada PR. CI lo gatea. |
| E2E tests | `make test-e2e` | ~3-5 min | Antes de merge a master, post cambios al pipeline de eventos o handlers. Requiere docker. |
| Cobertura HTML | `make coverage` | ~1 min | Cuando se quiere inspeccionar gaps de cobertura. |
| Smoke local | `../gesvial-sos-monitor-platform/scripts/smoke-local.sh` | <30s | Post `docker compose up -d` local. **Por crear en Fase 5 plan QA**. |
| Smoke prod | `../gesvial-sos-monitor-platform/scripts/smoke-prod.sh` | <60s | Post deploy a `apisosgw.gvops.cl`. **Por crear en Fase 5 plan QA**. |
| Benchmarks | `make benchmark` | varía | Ad-hoc; no se corre en CI por default. |

## Unit tests (`make test`)

```bash
cd gesvial-sos-monitor-server
make test
```

Equivalente a:
```bash
go test -race -shuffle=on -count=1 -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
```

- `-race`: detector de races en runtime.
- `-shuffle=on`: orden de tests aleatorio (detecta interdependencias).
- `-count=1`: deshabilita cache (necesario tras cambios al schema).
- `-covermode=atomic`: thread-safe para tests con goroutines.
- Genera `coverage.out` (consumible por `make coverage` o `go tool cover`).

**Pendientes Fase 5 plan QA** (cobertura ≥60% objetivo):
- `modules/events/service_test.go` — fan-out paralelo SSE+FCM, idempotencia, propagación de `event_id`.
- `modules/sse/service_test.go` — backpressure observable (retorna `ErrBufferFull` en lugar de drop).
- `modules/paneleventsbus/service_test.go` — idem.
- `modules/webhooks/dispatcher_test.go` — extender (hoy 4 tests; agregar firma HMAC en gesvial.15).
- `modules/push/service_test.go` — debounce, batch, fail-safe sin SA JSON.
- `internal/worker/db_retry_test.go` — Fase 1 plan QA (backoff exponencial 1s→30s, 20 intentos, honor `context.Done()`).

## E2E tests (`make test-e2e`)

```bash
cd gesvial-sos-monitor-server
make test-e2e
```

Equivalente a:
```bash
make test           # corre unit tests primero
cd test/e2e && go test -count=1 .
```

### Requisitos previos

- Docker corriendo localmente con permisos al socket.
- Puerto 4000 libre (el harness levanta su propio stack).
- `gesvial-sos-monitor-runtime/.env` configurado (la sesión del harness lee env vars como `PRIVATE_TOKEN`, `DB_PASSWORD`, `JWT_SECRET`).

### Estructura

Archivos vigentes (cloud-gesvial.22.1.2):

| Archivo | Cubre |
|---|---|
| `test/e2e/main_test.go` | Setup global del harness (start docker, wait healthy, teardown). |
| `test/e2e/utils_test.go` | Helpers de auth, HTTP, fixtures. **Refactor pendiente Fase 5**: extraer a `helpers.go` con `CommonSetup(t) *Harness`. |
| `test/e2e/clients_test.go` | Auth + tokens. |
| `test/e2e/device_selection_test.go` | Routing de eventos por `deviceId`. |
| `test/e2e/messages_test.go` | SMS lifecycle: enqueue → state transitions. |
| `test/e2e/mobile_test.go` | API mobile (devices, settings, tests, posts). |
| `test/e2e/priority_test.go` | Prioridades de mensajes. |
| `test/e2e/webhooks_test.go` | Webhooks user-registered + dispatcher (cloud-gesvial.14+). |

### Nuevos e2e tests a crear (Fase 5 plan QA)

- `test/e2e/event_flow_test.go` — PATCH `/message` → SSE evento + FCM enqueue + Webhook POST con mismo `event_id` en <2s.
- `test/e2e/worker_db_resilience_test.go` — `docker compose restart db` + log "worker started" en <90s. **Bloqueante Fase 1.**
- `test/e2e/sse_panel_test.go` — handshake SSE panel + ráfaga de eventos.
- `test/e2e/admin_scheduler_routing_test.go` — regresión `cloud-gesvial.22.1.1` (admin crea schedule → `user_id` real, no `__ADMIN__` sentinel).

## Lint (`make lint`)

```bash
cd gesvial-sos-monitor-server
make lint
```

Equivalente a `golangci-lint run --timeout=5m`. Config en `.golangci.yml` (raíz del repo server).

**Pendiente Fase 2 plan QA**: quitar `promlinter` del `.golangci.yml` cuando eliminemos Prometheus.

## Cobertura HTML (`make coverage`)

```bash
cd gesvial-sos-monitor-server
make coverage
```

Corre `make test` + genera `coverage.html`. Abrir en navegador para ver gaps por archivo. Para focus en un solo módulo:

```bash
go test -coverprofile=cov.out ./internal/sms-gateway/modules/events/...
go tool cover -html=cov.out
```

## Smoke local (post `docker compose up -d`)

**Estado**: por crear en Fase 5 plan QA. El script vivirá en `gesvial-sos-monitor-platform/scripts/smoke-local.sh`.

Forma esperada (alineada con `smoke-prod.sh`):

```bash
#!/usr/bin/env bash
set -eo pipefail
BASE="${BASE:-http://localhost:4000}"

check_health() {
    local r=$(curl -fsS "$BASE/health/ready" | jq -r '.status')
    [ "$r" = "Pass" ] || { echo "❌ health/ready=$r"; exit 1; }
}
# ... 6 checks en total
```

Para correrlo provisoriamente sin el script:

```bash
cd gesvial-sos-monitor-platform
docker compose up -d
sleep 30
curl -fsS http://localhost:4000/health/ready | jq      # Status: Pass
curl -fsS http://localhost:4000/health/live | jq       # Status: Pass
curl -fsS http://localhost:4000/health/startup | jq    # Status: Pass
docker compose logs --tail 20 server | grep -iE "ready|started|error|panic"
docker compose logs --tail 20 worker | grep -iE "started|error|connection refused"
docker compose ps        # los 3 servicios Up + healthy
```

## Smoke prod (post deploy a `apisosgw.gvops.cl`)

**Estado**: por crear en Fase 5 plan QA. El script vivirá en `gesvial-sos-monitor-platform/scripts/smoke-prod.sh`.

Provisorio:

```bash
curl -fsS https://apisosgw.gvops.cl/health/ready | jq                       # Pass
curl -fsS https://apisosgw.gvops.cl/health/live | jq                        # Pass
curl -fsS -X POST https://apisosgw.gvops.cl/api/3rdparty/v1/auth/token \
    -H "Authorization: Basic $(echo -n "$ADMIN_USER:$ADMIN_PASS" | base64)" \
    | jq -r '.token' | head -c 40 ; echo ...    # JWT obtenido
# ... + listar posts + listar schedules + GET /events/panel handshake + smoke webhook (con stub)
```

## Tests del repo `gesvial-sos-monitor-mobile/`

Fuera de scope de este documento. Ver `gesvial-sos-monitor-mobile/README.md` cuando se documente. Comando rápido:

```bash
cd gesvial-sos-monitor-mobile
./gradlew testReleaseUnitTest
./gradlew connectedAndroidTest   # requiere device/emulador
```

## CI

GitHub Actions workflows en `gesvial-sos-monitor-server/.github/workflows/`:
- `go.yml` — lint + test + coverage gate. **Pendiente Fase 5 plan QA**: subir baseline mínimo a 60%.
- (resto se documenta cuando se actualice el workflow file en Fase 5).

## Anti-patrones conocidos

- **No usar `make test` sin docker corriendo si los tests tocan repositories**. El cargador de fx puede fallar tarde (después de minutos de setup) con errores confusos. Verificar `docker compose ps` antes.
- **No mockear la DB en tests de repositorios o servicios que dependen de migraciones**. La verdad del esquema vive en `db/migrations/mysql/`; un mock se desincroniza silenciosamente cuando alguien agrega una columna.
- **No hacer `go test ./...` desde la raíz del monorepo viejo**. El working dir esperado es `gesvial-sos-monitor-server/`.

## Referencias

- Spec del sistema: [`gesvial-sos-monitor-platform/SYSTEM.md`](../../gesvial-sos-monitor-platform/SYSTEM.md)
- Event bus: [`EVENT_BUS.md`](./EVENT_BUS.md)
- Plan QA: [`gesvial-sos-monitor-platform/docs/plans/2026-05-17-qa-profesional.md`](../../gesvial-sos-monitor-platform/docs/plans/2026-05-17-qa-profesional.md) — Fase 5.
