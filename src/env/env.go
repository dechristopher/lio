package env

import "os"

// Env constants
type Env string

const (
	// Prod is the production environment at https://lioctad.org
	Prod Env = "prod"
	// Dev is the dev environment at https://dev.lioctad.org or localhost
	Dev Env = "dev"
)

// GetEnv returns the current environment
func GetEnv() Env {
	if os.Getenv("DEPLOY") == "prod" {
		return Prod
	}
	return Dev
}

// IsProd returns true if the current environment is production
func IsProd() bool {
	return GetEnv() == Prod
}

// IsDev returns true if the current environment is not production
func IsDev() bool {
	return !IsProd()
}
