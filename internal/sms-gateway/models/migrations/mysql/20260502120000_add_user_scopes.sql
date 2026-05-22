-- +goose Up
-- +goose StatementBegin
-- cloud-gesvial.22.0: scopes por usuario.
--
-- Hasta cloud-gesvial.21.x todos los usuarios humanos del panel recibían el
-- conjunto completo de scopes que el cliente pedía al login (incluyendo
-- `admin:all`). El operador del centro de monitoreo no debe poder borrar
-- postes ni cambiar contraseñas; pero el server NO discriminaba — el panel
-- ocultaba botones por convención, sin garantía de seguridad real.
--
-- Esta migración agrega una columna `scopes` (JSON serializado como TEXT)
-- en `users`. Cuando el handler `/auth/token` genera el JWT, intersecta los
-- scopes solicitados con los permitidos del usuario. Si el campo es NULL
-- (filas legacy), el comportamiento es identico al previo: se otorgan todos
-- los scopes pedidos. Cualquier valor explícito ACTÚA — usuarios nuevos
-- como `monitor` reciben sólo los scopes listados aunque el panel pida
-- admin:all.
--
-- Mantenemos el JSON serializado (no una tabla user_scopes con FK) porque
-- los scopes son un atributo del usuario y nunca se consultan por scope:
-- siempre es "leer scopes de Pedro al login". Una columna JSON minimiza
-- joins en el hot path.
ALTER TABLE `users`
    ADD COLUMN `scopes` TEXT NULL AFTER `password_hash`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `users` DROP COLUMN `scopes`;
-- +goose StatementEnd
