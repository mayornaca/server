# Panel SOS — Guía de uso (Administrador)

> Sistema en `https://apisosgw.gvops.cl`. Esta guía complementa la del operador. Las acciones marcadas con ⚙ requieren rol administrador.

## Iniciar sesión

Ingrese su usuario y contraseña, y pulse **Iniciar sesión**.

![Pantalla de login](img/01-login.png)

## Monitor en vivo

Esta es la pantalla principal: muestra el estado de cada poste, las pruebas en curso y la actividad reciente. La información se actualiza sola.

![Monitor en vivo](img/02-monitor.png)

## Probar varios postes a la vez

Abra **Postes SOS** desde el menú lateral. Filtre por nombre, kilómetro o estado con la barra superior si lo necesita.

![Lista de postes con filtros](img/03-posts-filtros.png)

Marque las casillas de los postes a probar, despliegue el menú **Tipo de prueba** y elija el tipo (SMS, Conectividad, Audio).

![Selección múltiple y dropdown de tipo](img/04-posts-seleccion.png)

Pulse **Probar seleccionados** y confirme con **Enviar** en el diálogo.

![Diálogo de confirmación batch](img/05-posts-confirm.png)

Aparece un mensaje con cuántas pruebas se crearon, cuáles ya estaban pendientes y cuántas fallaron al programarse.

![Resultado del envío](img/06-posts-toast.png)

## Ejecutar una prueba a un poste específico

En **Postes SOS**, pulse **Ver** en la fila del poste para abrir su detalle.

![Detalle de un poste](img/07-poste-detalle.png)

En la tarjeta **Ejecutar prueba**, seleccione el gateway (deje "Todos los gateways" si no está seguro) y el tipo de prueba, luego pulse **Ejecutar ahora**.

![Tarjeta Ejecutar prueba](img/08-poste-ejecutar.png)

Si el poste ya tiene una prueba pendiente, pulse **Cancelar pendiente y reintentar** para descartarla y enviar una nueva.

![Conflicto con prueba pendiente](img/09-poste-pendiente.png)

## Buscar pruebas históricas

Abra **Pruebas** desde el menú. Acote por tipo, estado, poste o rango de fechas usando los filtros superiores.

![Filtros del historial](img/10-tests-filtros.png)

Para ver la secuencia temporal, pulse **Línea de tiempo** en la esquina superior derecha.

![Vista línea de tiempo](img/11-tests-timeline.png)

Pulse cualquier fila para ver el detalle completo de la prueba, incluyendo evidencia y JSON crudo.

![Modal de detalle de test](img/12-test-modal.png)

## ⚙ Crear o editar un poste

En **Postes SOS**, pulse **+ Nuevo Poste** para añadir uno, o **Editar** en una fila para modificarlo. Complete nombre, kilómetro y teléfono, y pulse **Crear**.

![Diálogo de poste](img/14-poste-crear.png)

## ⚙ Desactivar o reactivar un poste

Abra el detalle del poste y pulse **Desactivar poste** (esquina superior derecha) para que deje de recibir pruebas programadas. Confirme con **Desactivar** en el diálogo. Para reactivarlo, pulse **Reactivar poste** en el mismo lugar.

![Desactivar poste](img/15-poste-desactivar.png)

## ⚙ Programar pruebas recurrentes

Abra **Configuración** desde el menú. La sección **Pruebas programadas** lista las reglas existentes con un interruptor para habilitarlas o pausarlas y los botones para editar o eliminar.

![Lista de reglas cron](img/16-settings-cron.png)

Pulse **+ Nueva regla**. Indique nombre, elija un preset o escriba la expresión cron (5 campos), defina tipo de prueba y gateway. Marque **Habilitada** y **Solo postes activos** según corresponda, y pulse **Crear**.

![Nueva regla cron](img/17-settings-cron-nueva.png)
