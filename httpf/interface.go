package httpf

import "gopkg.in/h2non/gentleman.v2"

type F interface {
	NewTopLevel() *gentleman.Client
}
