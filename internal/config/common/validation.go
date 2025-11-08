package common

import "fmt"

type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

type ValidationContext struct {
	Path string
}

func (c *ValidationContext) PushField(field string) *ValidationContext {
	if len(c.Path) == 0 {
		return &ValidationContext{Path: field}
	}

	return &ValidationContext{Path: c.Path + "." + field}
}

func (c *ValidationContext) PushIndex(i int) *ValidationContext {
	return &ValidationContext{Path: fmt.Sprintf("%s[%d]", c.Path, i)}
}

func (c *ValidationContext) NewError(message string) *ValidationError {
	return &ValidationError{Path: c.Path, Message: message}
}

func (c *ValidationContext) NewErrorf(format string, args ...interface{}) *ValidationError {
	return c.NewError(fmt.Sprintf(format, args...))
}

func (c *ValidationContext) NewErrorForField(field, message string) *ValidationError {
	return c.PushField(field).NewError(message)
}

func (c *ValidationContext) NewErrorfForField(field, format string, args ...interface{}) *ValidationError {
	return c.PushField(field).NewErrorf(format, args...)
}
