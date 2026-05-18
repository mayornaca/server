package fcm

import "errors"

var (
	ErrInitializationFailed = errors.New("initialization failed")
	// ErrNotInitialized indica que el Client no completó Open() — el SA JSON
	// no se cargó o falló parsing. Retornado por HealthCheck para que
	// /health/ready reporte Fail tempranamente. Fase 4 plan QA 2026-05-17.
	ErrNotInitialized = errors.New("fcm client not initialized")
)
