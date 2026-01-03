package auth

import (
	"github.com/gin-gonic/gin"
)

type PermissionValidatorBuilder struct {
	s        *service
	resource string
	verb     string
	idField  string
}

func (pb *PermissionValidatorBuilder) ForResource(resource string) *PermissionValidatorBuilder {
	pb.resource = resource
	return pb
}

func (pb *PermissionValidatorBuilder) ForVerb(verb string) *PermissionValidatorBuilder {
	pb.verb = verb
	return pb
}

func (pb *PermissionValidatorBuilder) ForIdField(idField string) *PermissionValidatorBuilder {
	pb.idField = idField
	return pb
}

func (pb *PermissionValidatorBuilder) Build() gin.HandlerFunc {
	if pb.resource == "" {
		panic("resource must be specified")
	}

	if pb.verb == "" {
		panic("verb must be specified")
	}

	// TODO: actual validation here
	return pb.s.Required()
}
