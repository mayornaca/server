package worker

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/capcom6/go-infra-fx/db"
	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// RetryConfig parametriza connectWithRetry. Los defaults de DefaultRetryConfig
// están calibrados para `docker compose restart db` típico (MariaDB tarda 5-15s
// en aceptar conexiones tras el restart) con margen de >5 min para casos de
// recovery más lentos.
type RetryConfig struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
}

// DefaultRetryConfig produce backoff exponencial 1s → 30s tope con 20 intentos.
// Total wallclock en el peor caso: ~5 min antes de fail-fast.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 20,
		InitialWait: 1 * time.Second,
		MaxWait:     30 * time.Second,
	}
}

// ProbeFunc valida una conexión. En producción es `db.PingContext`; en tests
// es una función fake controlable que simula transitorios.
type ProbeFunc func(ctx context.Context) error

// connectWithRetry ejecuta probe con backoff exponencial. Retorna nil si algún
// intento succeed; retorna error envolviendo el último fallo de probe si se
// agotan los intentos.
func connectWithRetry(ctx context.Context, probe ProbeFunc, cfg RetryConfig) error {
	var lastErr error
	wait := cfg.InitialWait
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := probe(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		wait *= 2
		if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}
	}
	return fmt.Errorf("connectWithRetry: exhausted %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// buildPingDSN construye un DSN MySQL desde los campos del db.Config de capdb
// para ser usado por el pre-check del worker. Importante: NO es el DSN que
// usará capdb internamente (capdb lo construye con su propio formato). Acá
// solo necesitamos algo que `sql.Open + PingContext` acepte para verificar
// reachability de la DB.
func buildPingDSN(c db.Config) (string, error) {
	loc := time.UTC
	if c.Timezone != "" {
		l, err := time.LoadLocation(c.Timezone)
		if err != nil {
			return "", fmt.Errorf("invalid timezone %q: %w", c.Timezone, err)
		}
		loc = l
	}
	mc := mysql.Config{
		User:                 c.User,
		Passwd:               c.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", c.Host, c.Port),
		DBName:               c.Database,
		Loc:                  loc,
		ParseTime:            true,
		AllowNativePasswords: true,
	}
	return mc.FormatDSN(), nil
}

// WaitForDB ejecuta sql.Open + PingContext con backoff exponencial via
// connectWithRetry sobre el shape de db.Config. Cierra la conexión al
// terminar (es probe-only; capdb abre la conexión productiva después).
//
// Llamado como fx.Invoke en worker.Run() para fail-fast si la DB no
// responde en ~5 min. Si succeed, capdb hace su sql.Open al primer
// intento sin retry porque la DB ya está lista.
func WaitForDB(c db.Config, logger *zap.Logger) error {
	dsn, err := buildPingDSN(c)
	if err != nil {
		return err
	}
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("sql.Open: %w", err)
	}
	defer sqlDB.Close()

	logger.Info("waiting for database",
		zap.String("host", c.Host),
		zap.Int("port", c.Port),
		zap.String("database", c.Database),
	)
	if err := connectWithRetry(context.Background(), sqlDB.PingContext, DefaultRetryConfig()); err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}
	logger.Info("database is ready")
	return nil
}
