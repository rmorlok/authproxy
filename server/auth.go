package server

// Based on https://github.com/katomaso/gin-auth/blob/v0.1.0/lib.go

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-pkgz/auth"
	"github.com/go-pkgz/auth/avatar"
	"github.com/go-pkgz/auth/logger"
	"github.com/go-pkgz/auth/token"
	"net/http"
	"time"
)

type Service struct {
	*auth.Service
}

func (s *Service) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		success := false
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			success = true
			if u, err := token.GetUserInfo(c.Request); err != nil {
				c.Set("user", u)
			}
			c.Next()
		})
		m := s.Middleware()
		m.Auth(_next).ServeHTTP(c.Writer, c.Request)
		if !success {
			c.Abort()
		}
	}
}

func (s *Service) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		_next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u, err := token.GetUserInfo(c.Request); err == nil {
				c.Set("user", u)
			}
			c.Next()
		})
		m := s.Middleware()
		m.Trace(_next).ServeHTTP(c.Writer, c.Request)
	}
}

func (s *Service) LoginHandler() gin.HandlerFunc {
	env := GetEnvironmentVariables()

	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, fmt.Sprintf("/auth/google/login?site=%s&from=%s", env.AuthUrl, env.AuthUrl))
	}
}

func (s *Service) AuthHandler() gin.HandlerFunc {
	authHandler, _ := s.Handlers()
	return func(c *gin.Context) {
		authHandler.ServeHTTP(c.Writer, c.Request)
	}
}

func (s *Service) AvatarHandler() gin.HandlerFunc {
	_, avatarProxy := s.Handlers()
	return func(c *gin.Context) {
		avatarProxy.ServeHTTP(c.Writer, c.Request)
	}
}

func GetAuthService() *Service {
	env := GetEnvironmentVariables()

	// define options
	options := auth.Opts{
		SecretReader: token.SecretFunc(func(id string) (string, error) { // secret key for JWT
			return env.AuthSecret, nil
		}),
		TokenDuration:  time.Minute * 5, // token expires in 5 minutes
		CookieDuration: time.Hour * 24,  // cookie expires in 1 day and will enforce re-login
		DisableXSRF:    true,            // Disable for now
		Issuer:         env.AuthIssuer,
		URL:            env.AuthUrl,
		Logger:         logger.Std,
		AvatarStore:    avatar.NewLocalFS("/tmp"),
	}

	// create auth service with providers
	service := auth.NewService(options)
	service.AddProvider("google", env.AuthGoogleClientId, env.AuthGoogleClientSecret)

	return &Service{
		service,
	}
}
