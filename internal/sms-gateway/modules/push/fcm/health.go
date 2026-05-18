package fcm

import "context"

// HealthCheck verifica que el FCM messaging client está inicializado
// (Open() corrió sin error). Retorna nil si está listo, o
// ErrNotInitialized si no — en cuyo caso /health/ready reportará Fail.
//
// Sin probe a Google: el cost de un round-trip a fcm.googleapis.com en cada
// /health/ready (que k8s/docker hacen cada 10-30s) sería desproporcionado.
// Si las credenciales caducaran u otro problema upstream, los logs de Send
// lo capturan — y los retries de push.Service amortiguan.
//
// Fase 4 plan QA 2026-05-17.
func (c *Client) HealthCheck(_ context.Context) error {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.initialized {
		return ErrNotInitialized
	}
	return nil
}

// markInitializedForTest es un helper interno SOLO para tests del package.
// Permite stub-inject que el client está "inicializado" sin requerir
// conectividad a Google ni un SA JSON real.
func (c *Client) markInitializedForTest() {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.initialized = true
}
