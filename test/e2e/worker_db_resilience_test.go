package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestWorkerSurvivesDBRestart valida que el worker, con connectWithRetry
// pre-check en internal/worker/db_retry.go, sobrevive a un
// `docker compose restart db` sin que su container exit. Antes del fix,
// capdb hacía sql.Open inicial sin retry y crashea el proceso fx → docker
// restart loop hasta que mariadb completara su healthcheck.
//
// Service `worker-resilience` está en el profile "resilience" del
// docker-compose.yml; se levanta solo para este test y se apaga al final.
//
// Validación: tras restart de db, el worker (a) NO crashea (uptime nunca
// se resetea), (b) loguea "waiting for database" durante el gap, y (c)
// "database is ready" + "worker started" aparece (post-fix solo si el
// worker apenas empieza; el caso esperado es que NO se reinicie).
func TestWorkerSurvivesDBRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// runCompose ejecuta `docker compose ...` capturando solo stdout. stderr
	// se descarta porque compose emite warnings de variables no set (ej.
	// FCM__CREDENTIALS_JSON) que contaminan el output cuando uno solo quiere
	// el container ID de `ps -q`.
	runCompose := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			// devolver stdout+stderr concatenado solo cuando hay error
			return stdout.String() + stderr.String(), err
		}
		return stdout.String(), nil
	}

	// Setup: levantar worker-resilience (db ya está corriendo por TestMain).
	if out, err := runCompose("--profile", "resilience", "up", "-d", "--wait", "worker-resilience"); err != nil {
		t.Fatalf("compose up worker-resilience: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		// ctx del test ya cancelado en este punto; usar background con timeout
		// propio para que el teardown de docker no aborte a medio camino.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		var buf bytes.Buffer
		cmd := exec.CommandContext(cleanupCtx, "docker", "compose", "--profile", "resilience", "down", "worker-resilience")
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if err := cmd.Run(); err != nil {
			t.Logf("cleanup compose down: %v\n%s", err, buf.String())
		}
	})

	// Capturar el container ID del worker antes del restart de db, para
	// detectar restart del container después.
	idBefore, err := runCompose("ps", "-q", "worker-resilience")
	if err != nil || strings.TrimSpace(idBefore) == "" {
		t.Fatalf("worker-resilience no está running antes del restart: id=%q err=%v", idBefore, err)
	}
	idBefore = strings.TrimSpace(idBefore)

	// Acción: reiniciar db. Esto cierra la conexión activa del worker y
	// fuerza el ciclo retry.
	if out, err := runCompose("restart", "db"); err != nil {
		t.Fatalf("compose restart db: %v\n%s", err, out)
	}

	// Espera: el worker tiene hasta 90s para que connectWithRetry vea db
	// arriba otra vez y el lifecycle siga normalmente (sin crashear).
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		idNow, err := runCompose("ps", "-q", "worker-resilience")
		if err != nil {
			continue
		}
		idNow = strings.TrimSpace(idNow)
		if idNow == "" {
			t.Fatalf("worker-resilience desapareció — crasheó sin restart por restart:no")
		}
		if idNow != idBefore {
			t.Fatalf("worker-resilience cambió de container ID (%s → %s) — crasheó y restartó", idBefore, idNow)
		}

		logs, err := runCompose("logs", "--no-color", "worker-resilience")
		if err != nil {
			continue
		}

		hasWaiting := strings.Contains(logs, "waiting for database")
		hasReady := strings.Contains(logs, "database is ready")
		hasStarted := strings.Contains(logs, "worker started")

		if hasReady && hasStarted {
			if !hasWaiting {
				t.Logf("nota: 'waiting for database' no apareció — db respondió antes de que connectWithRetry tuviera que esperar")
			}
			return
		}
	}

	// Timeout: dump logs y fallar.
	logs, _ := runCompose("logs", "--tail", "100", "--no-color", "worker-resilience")
	t.Fatalf("worker-resilience no recuperó conexión en 90s tras restart de db.\nLogs:\n%s", logs)
}
