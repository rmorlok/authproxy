---
title: Viewing S3 Blob Storage Data
---

The local Docker Compose stack uses SeaweedFS for S3-compatible storage of full
request/response logs. Its S3 endpoint is `http://localhost:9000`; it does not
publish an administrative console.

Use the AWS CLI to browse recorded blobs:

```bash
export AWS_ACCESS_KEY_ID=authproxy
export AWS_SECRET_ACCESS_KEY=authproxy-local-secret
export AWS_DEFAULT_REGION=us-east-1
aws --endpoint-url http://localhost:9000 s3 ls s3://authproxy-request-logs/ --recursive
aws --endpoint-url http://localhost:9000 s3 cp s3://authproxy-request-logs/OBJECT_KEY ./blob
```

These credentials are for local development only. The bucket requires
authentication. Integration tests use a separate instance on port 9003 and also
create an anonymous streaming-test bucket.

## Existing MinIO development data

SeaweedFS uses a new `seaweedfs_data` volume. It cannot read MinIO's data directory;
the previous `minio_data` volume is not migrated or deleted by this change. If you
need old recorded blobs, export them through the old server's S3 API and import
them into SeaweedFS before removing the old volume. Stop the old MinIO service
before starting SeaweedFS on the same port.

The persistent hosted demo continues to use its existing MinIO deployment;
its data migration is separate from the local and CI storage setup.
