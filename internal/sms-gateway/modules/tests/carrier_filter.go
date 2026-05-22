package tests

import (
	"encoding/json"
	"regexp"
	"strings"
)

// SMS evidence classifier (cloud-gesvial.20).
//
// A test SMS against an SOS post can produce up to two distinct asynchronous
// messages on the Z5 phone:
//
//   1. A carrier delivery receipt (Entel/Claro/Movistar/WOM) — confirms the
//      SMS reached the SIM (TRANSPORT layer).
//   2. A response from the SOS module's firmware containing battery / firmware
//      / network info — confirms the post is operational (APPLICATION layer).
//
// Pre-gesvial.20 the cloud collapsed both into a single status by demoting any
// carrier-shaped reply to FAILED("false positive"). gesvial.20 inverts the
// semantics: instead of demoting, the cloud CLASSIFIES each piece of evidence
// and stores both layers separately on the TestResult row
// (DeliveryConfirmed/DeliveryCarrier/DeliveryAt + Status from the post reply).
//
// This file is the cloud's authoritative classifier — replicated in the
// Android app (app-gesvial.16) for local UX progress, but the cloud version
// is the source of truth.

// IncomingType is the kind of message a sender/body pair represents.
type IncomingType string

const (
	IncomingTypeUnknown          IncomingType = "UNKNOWN"
	IncomingTypePostResponse     IncomingType = "POST_RESPONSE"
	IncomingTypeCarrierReceipt   IncomingType = "CARRIER_DELIVERY_RECEIPT"
)

// Classification is the result of analysing a (sender, body) pair.
// Carrier is non-empty only when Type=CARRIER_DELIVERY_RECEIPT and the carrier
// could be identified (e.g. "Entel", "Movistar", "Claro", "WOM", "VTR").
type Classification struct {
	Type    IncomingType
	Carrier string
}

//nolint:gochecknoglobals // compiled once, hot-path
var (
	// carrierIDPattern recognises Chilean mobile network operators by their
	// brand name. Matches as either the SMS sender or as a substring inside
	// the body for known phrasings.
	carrierIDPattern = regexp.MustCompile(`(?i)\b(entel|claro|movistar|wom|vtr|virgin\s*mobile)\b`)

	// Body patterns common to delivery receipts in Chile. Designed to be
	// permissive on phrasing variations (carriers tweak wording often).
	carrierBodyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)tu\s+sms.*(entreg|env)`),
		regexp.MustCompile(`(?i)tu\s+mensaje.*(entreg|env)`),
		regexp.MustCompile(`(?i)mensaje.*(entreg|env).*correctamente`),
		regexp.MustCompile(`(?i)delivery\s+(report|receipt)`),
		regexp.MustCompile(`(?i)sms\s+(entregado|enviado)`),
	}

	// Strong tokens that signal a real post response. If present in the body
	// the classifier short-circuits to POST_RESPONSE even if the body also
	// mentions a carrier name in a "Network: ENTEL PCS" field.
	postResponseTokens = []string{
		"battery voltage",
		"firmware version",
		"signal level",
		"network mode",
	}
)

// normaliseCarrier maps a regex match to a canonical carrier display name.
// Returns "" when the input does not match any known carrier.
func normaliseCarrier(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "entel":
		return "Entel"
	case "claro":
		return "Claro"
	case "movistar":
		return "Movistar"
	case "wom":
		return "WOM"
	case "vtr":
		return "VTR"
	}
	if strings.Contains(strings.ToLower(s), "virgin") {
		return "Virgin Mobile"
	}
	return ""
}

// Classify returns the Classification of an incoming message. Both arguments
// may be empty — empty inputs return UNKNOWN. The function never panics and
// is safe to call from request paths.
func Classify(sender, body string) Classification {
	if sender == "" && body == "" {
		return Classification{Type: IncomingTypeUnknown}
	}

	// Strong post-response tokens win over carrier identifiers — a real
	// post reply may contain "Network:ENTEL PCS" without being a carrier
	// receipt. Check this BEFORE the carrier path so we don't misclassify
	// a genuine response from a phone provisioned on the Entel network.
	loweredBody := strings.ToLower(body)
	for _, tok := range postResponseTokens {
		if strings.Contains(loweredBody, tok) {
			return Classification{Type: IncomingTypePostResponse}
		}
	}

	// Sender match — carrier names in the From field are the strongest
	// signal of a delivery receipt. Capture the carrier name for metadata.
	if sender != "" {
		if m := carrierIDPattern.FindString(sender); m != "" {
			return Classification{
				Type:    IncomingTypeCarrierReceipt,
				Carrier: normaliseCarrier(m),
			}
		}
	}

	// Body phrasing patterns — deliver-receipt language. Carrier name may or
	// may not appear in the body; if it does, capture it.
	for _, p := range carrierBodyPatterns {
		if p.MatchString(body) {
			carrier := ""
			if m := carrierIDPattern.FindString(body); m != "" {
				carrier = normaliseCarrier(m)
			}
			return Classification{
				Type:    IncomingTypeCarrierReceipt,
				Carrier: carrier,
			}
		}
	}

	return Classification{Type: IncomingTypeUnknown}
}

// IsCarrierConfirmation is the legacy boolean wrapper kept for backward
// compatibility with callers that only need to know "is this a delivery
// receipt?". New code should call Classify and inspect the full result so
// the carrier name and metadata can be stored.
func IsCarrierConfirmation(sender, body string) bool {
	return Classify(sender, body).Type == IncomingTypeCarrierReceipt
}

// extractResponseFromDetails parses the `details` JSON the gateway sends
// inside a TestResult and returns the sender and responseText if present.
// Returns empty strings if the JSON is malformed or the fields are absent —
// that's a signal to the caller (Report) to skip the carrier check.
//
// Expected shape from app-gesvial.13+:
//
//	{
//	  "phoneNumber": "+56966348648",
//	  "responseText": "Firmware Version:JR4G-...",
//	  "responseSender": "+56966348648",  // optional, falls back to phoneNumber
//	  ...
//	}
func extractResponseFromDetails(detailsJSON string) (sender, body string) {
	var details map[string]any
	if err := json.Unmarshal([]byte(detailsJSON), &details); err != nil {
		return "", ""
	}
	if v, ok := details["responseText"].(string); ok {
		body = v
	}
	if v, ok := details["responseSender"].(string); ok && v != "" {
		sender = v
	} else if v, ok := details["phoneNumber"].(string); ok {
		// Fallback: assume the response came from the post's own number. If
		// it didn't (carrier hijack), the body match below catches it.
		sender = v
	}
	return sender, body
}
