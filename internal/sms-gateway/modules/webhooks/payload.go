package webhooks

import (
	"time"

	"github.com/android-sms-gateway/client-go/smsgateway"
)

// DispatchEvent is the in-process envelope placed on the pubsub topic. It carries
// the minimum the consumer needs to look up webhooks and build the HTTP POST body.
type DispatchEvent struct {
	EventID  string                  `json:"event_id"`
	UserID   string                  `json:"userId"`
	DeviceID *string                 `json:"deviceId,omitempty"`
	Event    smsgateway.WebhookEvent `json:"event"`
	Payload  map[string]any          `json:"payload"`
}

// OutboundRequest is the JSON body the server POSTs to the user's webhook URL.
// Shape intentionally mirrors the Android gateway's payload so homologation
// receivers see equivalent messages from both sources.
//
// Fields follow smsgateway.SmsEventPayload.SmsSent/SmsDelivered/SmsFailed
// structure: messageId, phoneNumber, simNumber, and event-specific timestamps.
// The top-level envelope adds event name + deviceId so a single endpoint can
// demux without parsing the URL.
type OutboundRequest struct {
	WebhookID string                  `json:"webhookId"`
	DeviceID  *string                 `json:"deviceId,omitempty"`
	Event     smsgateway.WebhookEvent `json:"event"`
	Payload   map[string]any          `json:"payload"`
}

// SmsSentPayload returns the payload body for an sms:sent event.
func SmsSentPayload(messageID, recipient string, simNumber *int, partsCount int, sentAt time.Time) map[string]any {
	p := map[string]any{
		"messageId":   messageID,
		"phoneNumber": recipient,
		"recipient":   recipient,
		"partsCount":  partsCount,
		"sentAt":      sentAt.UTC().Format(time.RFC3339),
	}
	if simNumber != nil {
		p["simNumber"] = *simNumber
	}
	return p
}

// SmsDeliveredPayload returns the payload body for an sms:delivered event.
func SmsDeliveredPayload(messageID, recipient string, simNumber *int, deliveredAt time.Time) map[string]any {
	p := map[string]any{
		"messageId":   messageID,
		"phoneNumber": recipient,
		"recipient":   recipient,
		"deliveredAt": deliveredAt.UTC().Format(time.RFC3339),
	}
	if simNumber != nil {
		p["simNumber"] = *simNumber
	}
	return p
}

// SmsFailedPayload returns the payload body for an sms:failed event.
func SmsFailedPayload(messageID, recipient string, simNumber *int, failedAt time.Time, reason string) map[string]any {
	p := map[string]any{
		"messageId":   messageID,
		"phoneNumber": recipient,
		"recipient":   recipient,
		"failedAt":    failedAt.UTC().Format(time.RFC3339),
		"reason":      reason,
	}
	if simNumber != nil {
		p["simNumber"] = *simNumber
	}
	return p
}
