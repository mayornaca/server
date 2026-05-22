package models

import (
	"time"
)

type TimedModel struct {
	CreatedAt time.Time `gorm:"->;not null;autocreatetime:false;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt time.Time `gorm:"->;not null;autoupdatetime:false;default:CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)"`
}

type SoftDeletableModel struct {
	TimedModel

	DeletedAt *time.Time `gorm:"<-:update"`
}

type Device struct {
	SoftDeletableModel

	ID        string  `gorm:"primaryKey;type:char(21)"`
	Name      *string `gorm:"type:varchar(128)"`
	AuthToken string  `gorm:"not null;uniqueIndex;type:char(21)"`
	PushToken *string `gorm:"type:varchar(256)"`

	// cloud-gesvial.19.3 D7: gateway hardware/identity fields. Reportados por
	// la app Android en /api/mobile/v1/heartbeat (heartbeat liviano de
	// gesvial.18). El operador del panel los necesita para identificar
	// rápidamente a qué Z5 está mirando — el ID hash (`IYtJS5qQB...`) es
	// inservible como referencia humana.
	//
	// PhoneNumber: línea SIM del Z5. Obtenido en el dispositivo via
	// TelephonyManager.getLine1Number() (permiso READ_PHONE_NUMBERS, no
	// READ_PHONE_STATE — privilegio menor). Normalizado a E.164 (+56...).
	// NULL si la SIM no expone el número (algunos carriers ocultan
	// MSISDN incluso con permisos correctos — se queda NULL hasta que el
	// operador lo carga manualmente desde el panel).
	//
	// Model + OSVersion: identificación del hardware/software. Útil
	// cuando soporte tiene que distinguir el Z5 viejo del Z5 nuevo o
	// reproducir un bug en una versión específica de Android.
	PhoneNumber *string `gorm:"column:phone_number;type:varchar(20)"`
	Model       *string `gorm:"type:varchar(64)"`
	OSVersion   *string `gorm:"column:os_version;type:varchar(32)"`

	LastSeen time.Time `gorm:"not null;autocreatetime:false;default:CURRENT_TIMESTAMP(3);index:idx_devices_last_seen"`

	UserID string `gorm:"not null;type:varchar(32)"`
}

func NewDevice(name, pushToken *string) *Device {
	//nolint:exhaustruct // partial constructor
	return &Device{
		Name:      name,
		PushToken: pushToken,
	}
}

func (d *Device) IsEmpty() bool {
	if d == nil {
		return true
	}

	return d.ID == ""
}
