# Event Bus — Contrato del pipeline de eventos

Documento vivo (cloud-gesvial.22.1.2). Mantener en mismo commit que toque cualquier publisher / consumer / topic.

## Visión general

El servidor tiene **tres buses de eventos** distintos, cada uno con su público y su contrato:

| Bus | Implementación | Topic / Canal | Audiencia | Transporte saliente |
|---|---|---|---|---|
| **Mobile events** | `modules/events/Service` + `pkg/pubsub` | pubsub topic `"events"` | App Android (devices registrados) | SSE o FCM (excluyente — **a corregir Fase 3 plan QA**) |
| **Panel events** | `modules/paneleventsbus/Service` | in-process broadcast | Panel admin (browser) | SSE via `GET /api/3rdparty/v1/events/panel` |
| **Webhooks** | `modules/webhooks/Dispatcher` + `pkg/pubsub` | pubsub topic `"webhooks.dispatch"` | URLs externas registradas por el usuario | HTTP POST con retries `[5s, 15s, 45s]` |

Los tres conviven en el **mismo binario `server`** porque `PUBSUB__URL=memory://` por default — publisher y consumer deben compartir proceso. Para escalar horizontalmente cambiar a Redis (`PUBSUB__URL=redis://...`).

## Bus 1 — Mobile events

### Wire format

```go
// modules/events/event.go (no se incluye `event_id` todavía — pendiente Fase 2)
type Event struct {
    EventType smsgateway.PushEventType `json:"event"`
    Data      map[string]string        `json:"data,omitempty"`
}

type eventWrapper struct {
    UserID   string  `json:"userId"`
    DeviceID *string `json:"deviceId,omitempty"` // nil = broadcast a todos los devices del user
    Event    Event   `json:"event"`
}
```

### Publishers

| Evento | Constructor | Publisher (file:line) | Trigger |
|---|---|---|---|
| `MessageEnqueued` | `events.NewMessageEnqueuedEvent()` | `modules/messages/service.go:263` | Nuevo SMS encolado para envío (`Service.Enqueue`). |
| `WebhooksUpdated` | `events.NewWebhooksUpdatedEvent()` | `modules/webhooks/service.go` (varios sitios al modificar webhook) | CRUD sobre webhooks. |
| `MessagesExportRequested` | `events.NewMessagesExportRequestedEvent(since, until)` | (handler de exports) | Solicitud de export desde panel. |
| `SettingsUpdated` | `events.NewSettingsUpdatedEvent()` | `modules/settings/service.go:84` | Settings del device modificados (incluye CRUD de schedules — re-sincronización cron-side). |
| `TestRequested` | `events.NewTestRequestedEvent(testResultID, postID, testType)` | `modules/tests/service.go:868` y `:879` | Schedule individual o batch desde panel admin o cron dispatcher. |

**Pendiente Fase 3 plan QA**: las 4 invocaciones marcadas con `go func() { Notify(...) }()` en los publishers (messages:262-271, settings:84-88, tests:868-872, tests:879-888) son **goroutines sin await**. Si `Notify` falla, el caller no se entera. Reemplazar por `ctx, cancel := context.WithTimeout(callerCtx, 5*time.Second); defer cancel(); if err := s.eventsSvc.Notify(ctx, ...); err != nil { return err }`. Cambiar firma de `Notify` para aceptar `context.Context` como primer arg.

### Pipeline

```
[publisher.go: messages/settings/tests]
         │
         │  events.Service.Notify(userID, deviceID, event)
         ▼
[modules/events/service.go:54-82]
         │  serialize wrapper → pubsub.Publish(topic="events")
         ▼
[pubsub: memory:// in-process channel | Redis con N consumers]
         │
         ▼
[modules/events/service.go:84-110 Run()]
         │  Subscribe(topic="events") → for { Receive() → processEvent }
         ▼
[modules/events/service.go:130-189 processEvent]
         │  deviceSvc.Select(userID, deviceID filter)
         │  per device:
         │    ┌──────────────┐         ┌──────────────┐
         │    │ PushToken≠nil │   xor   │ no PushToken  │
         │    └──────┬───────┘         └──────┬───────┘
         │           ▼                        ▼
         │   pushSvc.Enqueue(token,event)  sseSvc.Send(deviceID,event)
         │
         ▼
[FCM via push/fcm/client.go]   |   [SSE via sse/service.go]
[upstream via push/upstream]   |
[noop via push/noop]           |
```

**Decisión arquitectural 3 (2026-05-17, pendiente Fase 3 plan QA)**: reemplazar el `xor` por fan-out paralelo con `sync.WaitGroup`. Logs por canal con `event_id`. SSE siempre intenta. FCM intenta si `device.PushToken != nil`. La app deduplica por `event_id` (nanoid de 21 chars).

### Backpressure y errores

- **SSE buffer 8 por conexión** (`modules/sse/service.go:18`). `Send()` con `select default` → **drop silencioso** + log warn + `metrics.IncrementConnectionErrors(ErrorTypeBufferFull)`. **Pendiente Fase 3**: retornar `ErrBufferFull` para que el caller decida si retry o dead-letter.
- **`events.Service.Run` con `safeProcessEvent`** y `defer recover()` para no tirar el loop si un handler panicquea (cloud-gesvial.19.1).
- Falla de FCM o SSE solo loguea + incrementa contador. No reintenta a nivel del bus.

## Bus 2 — Panel events (`paneleventsbus`)

### Wire format

```go
// modules/paneleventsbus/service.go:22-28
type Event struct {
    Type       EventType `json:"type"`
    UserID     string    `json:"userId,omitempty"`
    ResourceID string    `json:"resourceId,omitempty"`
    Status     string    `json:"status,omitempty"`
    At         time.Time `json:"at"`
}
```

### EventTypes definidos

| EventType | Publicado? | Publisher activo |
|---|---|---|
| `test.scheduled` | Sí | `tests/service.go:853` (en `ScheduleTest`) |
| `test.completed` | Sí | `tests/service.go:357,373` (en `Report` con override / merge) |
| `post.statusChanged` | Sí | `tests/service.go:1080` (en `updatePostStatus`) |
| `device.connected` | **No** | (definido pero sin publisher activo — pendiente hook en `devices.SetOnline`) |
| `device.disconnected` | **No** | (definido pero sin publisher activo — pendiente hook en `devices.MarkOffline`) |
| `schedule.fired` | **No** | (definido pero sin publisher activo — pendiente hook en `servertasks.cron_dispatcher`) |

### Pipeline

```
[publisher: tests/service.go]
         │
         │  panelBus.Publish(Event{Type, UserID, ResourceID, Status, At})
         ▼
[modules/paneleventsbus/service.go:71-84 Publish]
         │  s.mu.RLock() + for each subscriber: select { case sub.ch <- ev: } else { drop }
         │  Buffer 16, drop silencioso si lleno
         ▼
[N subscribers in-process]
         │  cada uno = una conexión SSE al panel (un cliente browser)
         ▼
[modules/sms-gateway/handlers/panelevents/3rdparty.go]
         │  GET /api/3rdparty/v1/events/panel (auth: hoistTokenFromQuery)
         │  Subscribe() → Receive() loop → escribe SSE al browser
         ▼
[Browser: EventSource en usePanelEvents hook]
```

### Backpressure

- Buffer 16 por subscriber, drop silencioso (línea 80-82 del service.go).
- **Pendiente Fase 3 plan QA**: retornar `ErrBufferFull` y log estructurado.

## Bus 3 — Webhooks server-side

### Wire format

```go
// modules/webhooks/dispatcher.go:105-110
type DispatchEvent struct {
    UserID   string                 `json:"userId"`
    DeviceID *string                `json:"deviceId,omitempty"`
    Event    smsgateway.WebhookEvent `json:"event"` // sms:sent | sms:delivered | sms:failed
    Payload  map[string]any         `json:"payload"`
}
```

### Publisher

| Evento | Publisher (file:line) | Trigger |
|---|---|---|
| `sms:sent` | `modules/messages/service.go` (en `UpdateState` con state `Sent`) | Gateway confirmó envío al carrier. |
| `sms:delivered` | `modules/messages/service.go` (en `UpdateState` con state `Delivered`) | Carrier confirmó entrega al destinatario. |
| `sms:failed` | `modules/messages/service.go` (en `UpdateState` con state `Failed`) | Carrier rechazó / timeout / error. |

### Pipeline

```
[messages.Service.UpdateState(messageID, state)]
         │
         │  webhooks.Dispatcher.Publish(ctx, userID, deviceID, event, payload)
         ▼
[modules/webhooks/dispatcher.go:96-127 Publish]
         │  if !cfg.ServerSideEnabled: return         ← feature flag OFF por default
         │  serialize DispatchEvent → pubsub.Publish(topic="webhooks.dispatch")
         ▼
[pubsub topic "webhooks.dispatch"]
         │
         ▼
[modules/webhooks/dispatcher.go: Dispatcher.Run]
         │  Subscribe → for { Receive() → dispatch }
         ▼
[per webhook URL registrado por el user]
         │  POST con backoff [5s, 15s, 45s] hasta `cfg.MaxRetries` (default 3)
         │  timeout `cfg.Timeout` (default 10s)
         │  Headers: Content-Type: application/json, User-Agent: gesvial-sosgw/...
         │  **Sin firma HMAC todavía** — paridad app↔server pendiente gesvial.15
```

### Feature flag

| Variable env | Default | Efecto |
|---|---|---|
| `WEBHOOKS__SERVER_SIDE_ENABLED` | `false` | Si `false`, `Publish()` es no-op y `Run()` retorna inmediato. La app Android sigue siendo el único webhook dispatcher. |
| `WEBHOOKS__TIMEOUT_SECONDS` | `10` | Timeout por POST. |
| `WEBHOOKS__MAX_RETRIES` | `3` | Reintentos antes de declarar dead. |

**Cómo activarlo en staging**: el operador enciende el flag, compara payloads recibidos en el endpoint user-registered (dos POSTs equivalentes por evento — uno de la app, uno del server). Cuando la paridad de body + headers + firma HMAC se confirma, release futuro deprecia el dispatcher de la app.

## Decisiones de diseño relevantes

1. **Tres buses, no uno solo**. El audience set difiere (devices Android vs browser admin vs URLs externas), el contrato de fiabilidad difiere (FCM/SSE best-effort vs HTTP con retries vs broadcast in-process), y el modelo de auth difiere. Unificarlos prematuramente sería un acoplamiento dañino.
2. **`paneleventsbus` es in-process broadcast, no pubsub**. Razón: el panel admin se accede solo desde el binario `server` (no del `worker`), y la latencia debe ser mínima. Para distribuir al binario `worker` se requiere Redis + reescribir `paneleventsbus` sobre `pubsub`.
3. **Drop silencioso vs back-pressure**: hoy todos los buses droppean si el consumer es lento (mejor perder un evento UI que congelar el cron). **Fase 3 plan QA** introduce observabilidad del drop (log + métrica).
4. **`event_id` correlation ID** (pendiente Fase 2 plan QA): cada evento publicado lleva nanoid 21 chars en `data.event_id`. Propagación:
   - SSE payload: `data: {"event_id":"<nanoid>",...}`
   - FCM body: `data.event_id`
   - Webhook header: `X-Event-Id: <nanoid>`
   - Logs Zap: `zap.String("event_id", id)`
   - Logcat app: `Timber.tag("event_id").i(id)`

   Permite `grep "<nanoid>" logs/*` + `adb logcat | grep "<nanoid>"` y ver el camino completo de un solo evento.

## Referencias

- Spec del sistema: [`gesvial-sos-monitor-platform/SYSTEM.md`](../../gesvial-sos-monitor-platform/SYSTEM.md) — sección "Decisiones arquitecturales vigentes (2026-05-17)" punto 3 y 4.
- Plan QA: [`gesvial-sos-monitor-platform/docs/plans/2026-05-17-qa-profesional.md`](../../gesvial-sos-monitor-platform/docs/plans/2026-05-17-qa-profesional.md) — Fases 2 y 3.
