package apierrors

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestSentinels_WireFormatSnapshot fija los códigos HTTP y mensajes de los
// errores expuestos para que cualquier drift accidental (alguien capitaliza
// "post" o cambia 404→400) sea detectado en CI antes del deploy. Estos
// strings llegan al panel React y al SDK Android — cambiarlos es breaking.
//
// Fase 6 plan QA 2026-05-17.
func TestSentinels_WireFormatSnapshot(t *testing.T) {
	cases := []struct {
		name    string
		err     *fiber.Error
		code    int
		message string
	}{
		{"ErrPostNotFound", ErrPostNotFound, fiber.StatusNotFound, "post not found"},
		{"ErrScheduleNotFound", ErrScheduleNotFound, fiber.StatusNotFound, "schedule not found"},
		{"ErrTestNotFound", ErrTestNotFound, fiber.StatusNotFound, "test not found"},
		{"ErrDeviceNotFound", ErrDeviceNotFound, fiber.StatusBadRequest, "invalid device id"},
		{"ErrInvalidLimit", ErrInvalidLimit, fiber.StatusBadRequest, "invalid limit"},
		{"ErrInvalidOffset", ErrInvalidOffset, fiber.StatusBadRequest, "invalid offset"},
		{"ErrInvalidBucket", ErrInvalidBucket, fiber.StatusBadRequest, "bucket must be 'day' or 'hour'"},
		{"ErrPostIDRequired", ErrPostIDRequired, fiber.StatusBadRequest, "postId is required"},
		{"ErrMessageContentMissing", ErrMessageContentMissing, fiber.StatusBadRequest, "no message content provided"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Code != tc.code {
				t.Errorf("code drift: got %d, want %d", tc.err.Code, tc.code)
			}
			if tc.err.Message != tc.message {
				t.Errorf("message drift: got %q, want %q", tc.err.Message, tc.message)
			}
		})
	}
}

// TestErrInvalidDateFrom_WrapsUnderlyingError valida que el constructor con
// argumento conserva el message del error wrapped — necesario para que el
// operador vea por qué falló el parseo (formato, valor inválido, etc.).
func TestErrInvalidDateFrom_WrapsUnderlyingError(t *testing.T) {
	underlying := errors.New("parse: not a date")
	got := ErrInvalidDateFrom(underlying)
	want := "invalid from: parse: not a date"
	if got.Code != fiber.StatusBadRequest {
		t.Errorf("code: got %d, want 400", got.Code)
	}
	if got.Message != want {
		t.Errorf("message: got %q, want %q", got.Message, want)
	}
}

func TestErrInvalidDateTo_WrapsUnderlyingError(t *testing.T) {
	underlying := errors.New("layout mismatch")
	got := ErrInvalidDateTo(underlying)
	want := "invalid to: layout mismatch"
	if got.Code != fiber.StatusBadRequest {
		t.Errorf("code: got %d, want 400", got.Code)
	}
	if got.Message != want {
		t.Errorf("message: got %q, want %q", got.Message, want)
	}
}
