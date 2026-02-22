package config

import (
	"context"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

type AwsCredentialsType string

const (
	AwsCredentialsTypeAccessKey AwsCredentialsType = "access_key"
	AwsCredentialsTypeImplicit  AwsCredentialsType = "implicit"
)

// AwsCredentialsImpl is the interface implemented by concrete blob storage credential configurations.
type AwsCredentialsImpl interface {
	GetCredentialsType() AwsCredentialsType
	GetAwsConfigLoadOptions(ctx context.Context) ([]func(*awsconfig.LoadOptions) error, error)
}

// AwsCredentials is the holder for a AwsCredentialsImpl instance.
type AwsCredentials struct {
	InnerVal AwsCredentialsImpl `json:"-" yaml:"-"`
}

func (c *AwsCredentials) GetCredentialsType() AwsCredentialsType {
	if c == nil || c.InnerVal == nil {
		return ""
	}
	return c.InnerVal.GetCredentialsType()
}

func (c *AwsCredentials) GetAwsConfigLoadOptions(ctx context.Context) ([]func(*awsconfig.LoadOptions) error, error) {
	if c == nil || c.InnerVal == nil {
		return nil, nil
	}

	return c.InnerVal.GetAwsConfigLoadOptions(ctx)
}

var _ AwsCredentialsImpl = (*AwsCredentials)(nil)
