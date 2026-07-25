package env

import "os"

// Env constants
type Env string

const (
	// Prod is the production environment at https://octad.gg
	Prod Env = "prod"
	// Dev is a non-production deployed environment. There is no live dev host
	// today; the value is retained for the DEPLOY=dev branch (see IsDev).
	Dev Env = "dev"
	// Local is the dev environment at localhost
	Local Env = "local"
)

// GetEnv returns the current environment
func GetEnv() Env {
	if os.Getenv("DEPLOY") == "prod" {
		return Prod
	} else if os.Getenv("DEPLOY") == "dev" {
		return Dev
	}
	return Local
}

// IsProd returns true if the current deployed environment is production
func IsProd() bool {
	return GetEnv() == Prod
}

// IsDev returns true if the current deployed environment is not production
func IsDev() bool {
	return GetEnv() == Dev
}

// IsLocal returns true if the current environment is not a
// cluster-deployed environment
func IsLocal() bool {
	return GetEnv() == Local
}
