package users

import (
	"fmt"

	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"gorm.io/gorm"
)

type userModel struct {
	models.SoftDeletableModel

	ID           string `gorm:"primaryKey;type:varchar(32)"`
	PasswordHash string `gorm:"not null;type:varchar(72)"`
	// Scopes — cloud-gesvial.22.0.1. JSON array de scopes permitidos para
	// este usuario, e.g. `["posts:list","posts:read","tests:list"]`. NULL
	// = compat legacy (todos los scopes solicitados se conceden).
	//
	// Patrón consistente con el resto del proyecto: pointer-to-string +
	// `gorm:"type:text"` (idéntico al `Name *string \`gorm:"type:varchar(128)"\``
	// del modelo `Device`). La columna SQL la creó la migración goose
	// `20260502120000_add_user_scopes.sql`; AutoMigrate del userModel
	// inspecciona y la deja como está si el tipo coincide.
	Scopes *string `gorm:"type:text"`
}

func newUserModel(id string, passwordHash string) *userModel {
	//nolint:exhaustruct // partial constructor
	return &userModel{
		ID:           id,
		PasswordHash: passwordHash,
	}
}

func (u *userModel) TableName() string {
	return "users"
}

// Migrate sincroniza el schema de `users`. Goose corre antes y crea las
// columnas; AutoMigrate sirve para entornos dev sin goose y para validar
// que el struct matchea la tabla.
//
// cloud-gesvial.22.0.x historial — el bug "unsupported data type: &[]:
// Table not set" que rompía `schedules.FetchDue`, `tests.ExpirePending` y
// `tests.RetryFailed` NO estaba en este Migrate. Estaba en `domain.User`:
// el campo `AllowedScopes []string` no tenía `gorm:"-"` y, vía la relación
// FK `posts.SosPost.User`, GORM lo intentaba mapear como columna slice.
// Fix definitivo en 22.0.3 — ver `domain.go`. Este Migrate vuelve al patrón
// simple del resto del proyecto.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(new(userModel)); err != nil {
		return fmt.Errorf("users migration failed: %w", err)
	}
	return nil
}
