package config

import (
	"context"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// AwsCredentialsImplicit uses the default AWS credential chain
// (environment variables, shared credentials file ~/.aws/credentials, IAM roles, etc.).
type AwsCredentialsImplicit struct {
	Type AwsCredentialsType `json:"type" yaml:"type"`
}

func (c *AwsCredentialsImplicit) GetCredentialsType() AwsCredentialsType {
	return AwsCredentialsTypeImplicit
}

func (c *AwsCredentialsImplicit) GetAwsConfigLoadOptions(_ context.Context) ([]func(*awsconfig.LoadOptions) error, error) {
	// Use the default AWS credential chain (env vars, ~/.aws/credentials, IAM roles, etc.)
	// No additional configuration needed.
	return nil, nil
}
