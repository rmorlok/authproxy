package apblob

import (
	"bytes"
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	pkgerrors "github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

type s3Client struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3Client creates a new S3-based blob storage client from the given config.
func NewS3Client(ctx context.Context, cfg *config.BlobStorageS3) (Client, error) {
	opts, err := cfg.GetAwsConfigLoadOptions(ctx)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to get AWS config load options for blob storage")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to load AWS config for blob storage")
	}

	s3Opts := cfg.GetS3Options()
	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &s3Client{
		client: client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
	}, nil
}

func (c *s3Client) key(key string) string {
	return c.prefix + key
}

func (c *s3Client) Put(ctx context.Context, input PutInput) error {
	s3Input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(c.key(input.Key)),
		Body:        bytes.NewReader(input.Data),
		ContentType: input.ContentType,
		Expires:     input.ExpiresAt,
	}

	_, err := c.client.PutObject(ctx, s3Input)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to put blob to S3")
	}

	return nil
}

func (c *s3Client) Get(ctx context.Context, key string) ([]byte, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.key(key)),
	}

	result, err := c.client.GetObject(ctx, input)
	if err != nil {
		var nsk *types.NoSuchKey
		if pkgerrors.As(err, &nsk) {
			return nil, ErrBlobNotFound
		}
		// Also check for the generic not found case (some S3-compatible stores)
		var notFound *types.NotFound
		if pkgerrors.As(err, &notFound) {
			return nil, ErrBlobNotFound
		}
		return nil, pkgerrors.Wrap(err, "failed to get blob from S3")
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to read blob body from S3")
	}

	return data, nil
}

func (c *s3Client) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.key(key)),
	}

	_, err := c.client.DeleteObject(ctx, input)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to delete blob from S3")
	}

	return nil
}

var _ Client = (*s3Client)(nil)
