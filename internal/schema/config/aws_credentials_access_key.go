package config

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// AwsCredentialsAccessKey provides explicit access key credentials for S3-compatible storage.
type AwsCredentialsAccessKey struct {
	Type            AwsCredentialsType `json:"type" yaml:"type"`
	AccessKeyID     *StringValue       `json:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey *StringValue       `json:"secret_access_key" yaml:"secret_access_key"`
}

func (c *AwsCredentialsAccessKey) GetCredentialsType() AwsCredentialsType {
	return AwsCredentialsTypeAccessKey
}

func (c *AwsCredentialsAccessKey) GetAwsConfigLoadOptions(ctx context.Context) ([]func(*awsconfig.LoadOptions) error, error) {
	opts := make([]func(*awsconfig.LoadOptions) error, 0)

	if c.AccessKeyID != nil && c.SecretAccessKey != nil {
		accessKey, err := c.AccessKeyID.GetValue(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve blob storage access key ID: %w", err)
		}
		secretKey, err := c.SecretAccessKey.GetValue(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve blob storage secret access key: %w", err)
		}
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	return opts, nil
}
