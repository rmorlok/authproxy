package httpf

import "github.com/h2non/gentleman"

type F interface {
	NewTopLevel() *gentleman.Client
}
