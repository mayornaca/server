package e2e

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func readAllClose(r *http.Response) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// TestEventFlow_MessageEnqueuedSSECarriesEventID valida el end-to-end del
// event pipeline (Fase 5 plan QA 2026-05-17): POST a /3rdparty/v1/messages
// dispara MessageEnqueued event → events.Service.Notify → SSE delivery al
// device con `event_id` (nanoid 21) en el payload. Es la versión SSE de la
// propagación end-to-end; el path webhook está cubierto por
// `webhooks/dispatcher_test.go::TestDispatcher_PropagatesEventIDAsHeader`.
//
// Tolerancia SLO: <2s desde POST hasta recibir el SSE event.
//
// Nota técnica: el SSE handler de fiber no flushea headers hasta el primer
// write del stream — eso ocurre cuando llega un event. Por eso conectamos
// en goroutine y POSTeamos después de un pequeño delay para que la
// suscripción esté registrada en el server.
func TestEventFlow_MessageEnqueuedSSECarriesEventID(t *testing.T) {
	device := mobileDeviceRegister(t, publicMobileClient)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	type sseResult struct {
		data    map[string]string
		latency time.Duration
		err     error
	}
	resultCh := make(chan sseResult, 1)

	// Goroutine: conectar al SSE stream + leer hasta encontrar event con event_id.
	go func() {
		sseURL := "http://localhost:3000/api/mobile/v1/events"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseURL, nil)
		if err != nil {
			resultCh <- sseResult{err: err}
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+device.Token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			resultCh <- sseResult{err: fmt.Errorf("connect SSE: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			resultCh <- sseResult{err: fmt.Errorf("SSE handshake: expected 200, got %d", resp.StatusCode)}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			raw := strings.TrimPrefix(line, "data: ")
			var data map[string]string
			if jsonErr := json.Unmarshal([]byte(raw), &data); jsonErr != nil {
				continue
			}
			if _, ok := data["event_id"]; ok {
				resultCh <- sseResult{data: data, latency: time.Since(time.Now())}
				return
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			resultCh <- sseResult{err: fmt.Errorf("scan: %w", scanErr)}
			return
		}
		resultCh <- sseResult{err: fmt.Errorf("stream closed without event_id")}
	}()

	// Dar tiempo a que el SSE register la conexión en el server.
	time.Sleep(500 * time.Millisecond)

	// POST mensaje via 3rdparty API → dispara MessageEnqueued event al device.
	// id (max 36 chars), phoneNumber válido E.164, textMessage con text.
	auth := base64.StdEncoding.EncodeToString([]byte(device.Login + ":" + device.Password))
	msgBody := fmt.Sprintf(
		`{"id":"evt-%d","textMessage":{"text":"flow test"},"phoneNumbers":["+56987654321"],"deviceId":"%s"}`,
		time.Now().Unix(), device.ID,
	)
	msgURL := "http://localhost:3000/api/3rdparty/v1/messages"
	msgReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, msgURL, strings.NewReader(msgBody))
	msgReq.Header.Set("Content-Type", "application/json")
	msgReq.Header.Set("Authorization", "Basic "+auth)
	msgResp, err := http.DefaultClient.Do(msgReq)
	if err != nil {
		t.Fatalf("POST message: %v", err)
	}
	respBody, _ := readAllClose(msgResp)
	if msgResp.StatusCode >= 400 {
		t.Fatalf("POST message returned %d: %s", msgResp.StatusCode, respBody)
	}
	postedAt := time.Now()

	// Esperar event con event_id en <2s.
	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatal(res.err)
		}
		eventID := res.data["event_id"]
		latency := time.Since(postedAt)
		if eventID == "" {
			t.Errorf("event_id empty in SSE data: %v", res.data)
		}
		if len(eventID) != 21 {
			t.Errorf("event_id should be nanoid 21 chars, got %d: %q", len(eventID), eventID)
		}
		if latency > 2*time.Second {
			t.Errorf("SLO violation: event arrived in %v, expected <2s", latency)
		}
		t.Logf("event_id %q received in %v", eventID, latency)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SSE event with event_id (SLO: <2s)")
	}
}
