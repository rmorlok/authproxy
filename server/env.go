package server

import (
	"github.com/joho/godotenv"
	"os"
	"strings"
	"sync"
)

type EnvironmentVariables struct {
	AuthSecret             string
	AuthIssuer             string
	AuthUrl                string
	AuthGoogleClientId     string
	AuthGoogleClientSecret string
	CorsAllowedOrigins     []string
}

var envOnce sync.Once
var envVariables *EnvironmentVariables

func GetEnvironmentVariables() *EnvironmentVariables {
	envOnce.Do(func() {
		// optionally load a .env file
		_ = godotenv.Load()

		envVariables = &EnvironmentVariables{
			AuthSecret:             os.Getenv("AUTH_SECRET"),
			AuthIssuer:             os.Getenv("AUTH_ISSUER"),
			AuthUrl:                os.Getenv("AUTH_URL"),
			AuthGoogleClientId:     os.Getenv("AUTH_GOOGLE_CLIENT_ID"),
			AuthGoogleClientSecret: os.Getenv("AUTH_GOOGLE_CLIENT_SECRET"),
			CorsAllowedOrigins:     strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ","),
		}
	})

	return envVariables
}
