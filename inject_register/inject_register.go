package inject_register

import adminApisCtrl "github.com/theseed-labs/os-backend/internal_inject/user/admin_apis"

func RegisterAll() {
	// Register your services here

	adminApisCtrl.Register()
}
