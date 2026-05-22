package users

import (
	"encoding/json"
	"time"
)

type User struct {
	ID string

	// AllowedScopes son los scopes que este usuario puede recibir en un JWT.
	// nil = compat legacy: cualquier scope solicitado se concede.
	// []string vacío explícito = ningún scope (no puede generar tokens útiles).
	// cloud-gesvial.22.0.
	//
	// `gorm:"-"` es OBLIGATORIO. Este struct se referencia desde `posts.SosPost.User`
	// con `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`. Cuando GORM
	// introspecciona SosPost para resolver la relación FK, walks por los campos
	// de User y los trata como columnas. Sin el tag, `AllowedScopes []string`
	// se interpreta como una columna SQL slice y rompe queries posteriores con
	// `unsupported data type: &[]: Table not set` en otros repos. cloud-gesvial.22.0.3.
	AllowedScopes []string `gorm:"-"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func newUser(model *userModel) *User {
	u := &User{
		ID: model.ID,

		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
	// Si la columna `scopes` está poblada, parsearla. JSON inválido cae a nil
	// (compat legacy) — preferimos degradar en lugar de bloquear el login por
	// data corrupta en BD.
	if model.Scopes != nil && *model.Scopes != "" {
		var scopes []string
		if err := json.Unmarshal([]byte(*model.Scopes), &scopes); err == nil {
			u.AllowedScopes = scopes
		}
	}
	return u
}
