package events

import (
	"time"

	"github.com/android-sms-gateway/client-go/smsgateway"
)

func NewMessageEnqueuedEvent() Event {
	return NewEvent(smsgateway.PushMessageEnqueued, nil)
}

func NewWebhooksUpdatedEvent() Event {
	return NewEvent(smsgateway.PushWebhooksUpdated, nil)
}

func NewMessagesExportRequestedEvent(since, until time.Time) Event {
	return NewEvent(
		smsgateway.PushMessagesExportRequested,
		map[string]string{
			"since": since.Format(time.RFC3339),
			"until": until.Format(time.RFC3339),
		},
	)
}

func NewSettingsUpdatedEvent() Event {
	return NewEvent(smsgateway.PushSettingsUpdated, nil)
}

func NewTestRequestedEvent(testResultID, postID, testType string) Event {
	return NewEvent(
		smsgateway.PushEventType("TestRequested"),
		map[string]string{
			"testResultId": testResultID,
			"postId":       postID,
			"testType":     testType,
		},
	)
}
