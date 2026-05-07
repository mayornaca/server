package converters

import (
	"time"

	"github.com/android-sms-gateway/client-go/smsgateway"
	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/capcom6/go-helpers/anys"
)

func DeviceToDTO(device models.Device) smsgateway.Device {
	return smsgateway.Device{
		ID:        device.ID,
		Name:      anys.OrDefault(device.Name, ""),
		CreatedAt: device.CreatedAt,
		UpdatedAt: device.UpdatedAt,
		DeletedAt: device.DeletedAt,
		LastSeen:  device.LastSeen,
	}
}

// DeviceWithIdentity extends the upstream smsgateway.Device DTO with the
// human-readable identity fields added in cloud-gesvial.19.3 (phone number,
// hardware model, OS version, FCM availability). The upstream DTO is kept
// untouched so the legacy capcom6 client SDK keeps deserialising correctly;
// the panel React frontend reads the extra fields directly from JSON.
type DeviceWithIdentity struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
	LastSeen    time.Time  `json:"lastSeen"`
	PhoneNumber *string    `json:"phoneNumber,omitempty"`
	Model       *string    `json:"model,omitempty"`
	OSVersion   *string    `json:"osVersion,omitempty"`
	HasPushToken bool      `json:"hasPushToken"`
}

// DeviceToIdentityDTO is the converter used by the panel-facing GET /devices
// endpoint. cloud-gesvial.19.3 D7. We intentionally do NOT echo the FCM
// `push_token` itself — exposing it would let any panel reader re-send pushes
// to that gateway. We just expose `hasPushToken` so the panel can render the
// channel badge ("FCM" vs "SSE") that explains how the cloud reaches the Z5.
func DeviceToIdentityDTO(device models.Device) DeviceWithIdentity {
	return DeviceWithIdentity{
		ID:           device.ID,
		Name:         anys.OrDefault(device.Name, ""),
		CreatedAt:    device.CreatedAt,
		UpdatedAt:    device.UpdatedAt,
		DeletedAt:    device.DeletedAt,
		LastSeen:     device.LastSeen,
		PhoneNumber:  device.PhoneNumber,
		Model:        device.Model,
		OSVersion:    device.OSVersion,
		HasPushToken: device.PushToken != nil && *device.PushToken != "",
	}
}
