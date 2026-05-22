package e2e

import (
	"encoding/json"
	"testing"

	"github.com/go-resty/resty/v2"
)

// TestHealthReady_IncludesPubsubAndPushChecks valida end-to-end que
// `/health/ready` reporta los nuevos checks de Fase 4 plan QA 2026-05-17:
// - `pubsub:roundtrip`: round-trip publish→subscribe en <500ms.
// - `push:init`: backend de push inicializado (noop en este test environment).
//
// Smoke equivalente al `curl http://localhost:4000/health/ready | jq` del
// plan, ejecutado contra el container public del harness.
func TestHealthReady_IncludesPubsubAndPushChecks(t *testing.T) {
	// HealthHandler está registrado en raíz (app.Use) — no bajo /api. Por
	// eso usamos host:port directo en lugar de PublicURL (que incluye /api).
	res, err := resty.New().
		SetBaseURL("http://localhost:3000").
		R().
		Get("/health/ready")
	if err != nil {
		t.Fatalf("GET /health/ready: %v", err)
	}
	if !res.IsSuccess() {
		t.Fatalf("expected 2xx, got %d: %s", res.StatusCode(), res.String())
	}

	var body struct {
		Status string                    `json:"status"`
		Checks map[string]map[string]any `json:"checks"`
	}
	if err := json.Unmarshal(res.Body(), &body); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, res.String())
	}

	requiredChecks := []string{"pubsub:roundtrip", "push:init"}
	for _, key := range requiredChecks {
		check, ok := body.Checks[key]
		if !ok {
			t.Errorf("missing check %q in response. got checks: %v", key, mapKeys(body.Checks))
			continue
		}
		if status, _ := check["status"].(string); status != "pass" {
			t.Errorf("check %q expected status=pass, got %v. detail: %v", key, check["status"], check)
		}
	}

	if body.Status != "pass" {
		t.Errorf("overall status expected pass, got %q", body.Status)
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
