package httpf

import "gopkg.in/h2non/gentleman.v2"

//go:generate mockgen -source=./interface.go -destination=./mock/httpf.go -package=mock
type F interface {
	NewTopLevel() *gentleman.Client
}
