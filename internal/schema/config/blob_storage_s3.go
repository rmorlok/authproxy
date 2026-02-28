package config

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type BlobStorageS3 struct {
	Provider       BlobStorageProvider `json:"provider" yaml:"provider"`
	Endpoint       string              `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Region         string              `json:"region,omitempty" yaml:"region,omitempty"`
	Bucket         string              `json:"bucket" yaml:"bucket"`
	Credentials    *AwsCredentials     `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	ForcePathStyle bool                `json:"force_path_style,omitempty" yaml:"force_path_style,omitempty"`
	Prefix         string              `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

func (b *BlobStorageS3) GetProvider() BlobStorageProvider {
	return BlobStorageProviderS3
}

func (b *BlobStorageS3) GetAwsConfigLoadOptions(ctx context.Context) ([]func(*awsconfig.LoadOptions) error, error) {
	opts, err := b.Credentials.GetAwsConfigLoadOptions(ctx)
	if err != nil {
		return nil, err
	}

	if b.Region != "" {
		opts = append(opts, awsconfig.WithRegion(b.Region))
	}

	return opts, nil
}

func (b *BlobStorageS3) GetS3Options() []func(*s3.Options) {
	s3Opts := make([]func(*s3.Options), 0)

	if b.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(b.Endpoint)
		})
	}

	if b.ForcePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return s3Opts
}
