package main

import (
	"os"

	"github.com/android-sms-gateway/server/internal/health"
	smsgateway "github.com/android-sms-gateway/server/internal/sms-gateway"
	"github.com/android-sms-gateway/server/internal/worker"
)

const (
	cmdWorker = "worker"
	cmdHealth = "health"
)

//	@securitydefinitions.basic	ApiAuth
//	@description				User authentication

//	@securitydefinitions.apikey	JWTAuth
//	@in							header
//	@name						Authorization
//	@description				JWT authentication

//	@securitydefinitions.apikey	UserCode
//	@in							header
//	@name						Authorization
//	@description				User one-time code authentication

//	@securitydefinitions.apikey	MobileToken
//	@in							header
//	@name						Authorization
//	@description				Mobile device token

//	@securitydefinitions.apikey	ServerKey
//	@in							header
//	@name						Authorization
//	@description				Private server authentication

//	@title			Gesvial SOS Gateway API
//	@version		{APP_VERSION}
//	@description	API del servidor de Gesvial para monitoreo de postes SOS de autopistas. Fork de capcom6/sms-gateway. Provee endpoints para gestión de postes, pruebas programadas, devices (gateways Android), webhooks y health checks.

//	@contact.name	Gesvial
//	@contact.url	https://apisosgw.gvops.cl/

//	@license.name	Apache 2.0
//	@license.url	https://www.apache.org/licenses/LICENSE-2.0

//	@host		apisosgw.gvops.cl
//	@schemes	https
//
// SMSGate Backend.
func main() {
	args := os.Args[1:]
	cmd := "start"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case cmdHealth:
		health.Run()
		return
	case cmdWorker:
		worker.Run()
		return
	}

	smsgateway.Run()
}
