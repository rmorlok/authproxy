package admin_api

import (
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/logger"
	"time"
)

// Based on https://github.com/katomaso/gin-auth/blob/v0.1.0/lib.go

type Service struct {
	*auth.Service
}

func GetAuthService() *Service {

	// define options
	options := auth.Opts{
		SecretReader: auth.SecretFunc(func(id string) (string, error) { // secret key for JWT
			return "some-secret", nil
		}),
		TokenDuration:  time.Minute * 5, // token expires in 5 minutes
		CookieDuration: time.Hour * 24,  // cookie expires in 1 day and will enforce re-login
		DisableXSRF:    true,            // Disable for now
		Issuer:         "tmp-issuer",
		Logger:         logger.Std,
	}

	// create auth service with providers
	service := auth.NewService(options)

	return &Service{
		service,
	}
}
