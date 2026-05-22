// Package apierrors centralizes the *fiber.Error sentinels used across the
// HTTP handlers so the wire-format messages stay consistent and grep-able.
//
// Pre-Fase 6 plan QA (2026-05-17): the same string ("post not found",
// "schedule not found", etc.) was duplicated up to 3 times per handler;
// any drift between duplicates was invisible to readers and operators.
// Now each handler imports this package and returns the shared sentinel.
//
// Lives in a sub-package (not in `handlers` root) to avoid circular imports
// since `handlers` root imports each sub-package (posts, schedules, tests).
package apierrors

import "github.com/gofiber/fiber/v2"

// Resource-not-found family (HTTP 404). Use as `return apierrors.ErrPostNotFound`.
var (
	ErrPostNotFound     = fiber.NewError(fiber.StatusNotFound, "post not found")
	ErrScheduleNotFound = fiber.NewError(fiber.StatusNotFound, "schedule not found")
	ErrTestNotFound     = fiber.NewError(fiber.StatusNotFound, "test not found")
	ErrDeviceNotFound   = fiber.NewError(fiber.StatusBadRequest, "invalid device id")
)

// Query-string validation family (HTTP 400). Cover pagination and time-window
// filters shared across `GET /tests` and `GET /tests/history`.
var (
	ErrInvalidLimit   = fiber.NewError(fiber.StatusBadRequest, "invalid limit")
	ErrInvalidOffset  = fiber.NewError(fiber.StatusBadRequest, "invalid offset")
	ErrInvalidBucket  = fiber.NewError(fiber.StatusBadRequest, "bucket must be 'day' or 'hour'")
	ErrPostIDRequired = fiber.NewError(fiber.StatusBadRequest, "postId is required")
)

// Body/payload validation (HTTP 400).
var (
	ErrMessageContentMissing = fiber.NewError(fiber.StatusBadRequest, "no message content provided")
)

// ErrInvalidDateFrom wraps the parser error for the `from` query param.
// Centralizing the prefix keeps log grep deterministic across handlers.
func ErrInvalidDateFrom(err error) *fiber.Error {
	return fiber.NewError(fiber.StatusBadRequest, "invalid from: "+err.Error())
}

// ErrInvalidDateTo wraps the parser error for the `to` query param.
func ErrInvalidDateTo(err error) *fiber.Error {
	return fiber.NewError(fiber.StatusBadRequest, "invalid to: "+err.Error())
}
