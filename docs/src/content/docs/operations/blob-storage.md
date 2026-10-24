---
title: Viewing S3 Blob Storage Data
description: Browse the SeaweedFS buckets that hold full request/response logs locally and in the hosted demo.
---

AuthProxy stores full request/response logs as objects in S3-compatible blob
storage. Both the local Docker Compose stack and the hosted demo use
[SeaweedFS](https://github.com/seaweedfs/seaweedfs) for this, with recorded
requests in the `authproxy-request-logs` bucket.

Each object is keyed `<namespace>/<request-id>.enc` and holds the log entry
encrypted with that namespace's encryption key. Browsing the bucket shows which
requests were recorded and how large they are. To read a request's contents,
open it in the Admin UI's request detail view, which decrypts it for you.

## When blob storage is unavailable

Request summaries live in the request-events database, not in blob storage.
If a blob is missing or blob storage returns an error,
`GET /metrics/request-events/{id}` still returns `200` with the summary in
`spec`, leaves out `spec.capture`, and reports why in `status`:

| `status.captureState` | Meaning |
|---|---|
| `loaded` | Captured details were read successfully. |
| `missing` | Blob storage has no object for this request. |
| `unavailable` | Blob storage failed to return the object, or it could not be decrypted. |

The Admin UI shows a notice above the request detail in either degraded
state. The underlying error is not returned to clients. To track it:

- **Logs:** look for `failed to load full request log from blob storage`. It is
  logged at `ERROR` for `unavailable` and `WARN` for `missing`, with
  `request_id`, `namespace`, and `capture_state` attributes.
- **Traces:** the request's span records the error as an exception event and
  sets `authproxy.request_event.capture_state`.

In Grafana Explore, for example:

```logql
{service_name=~"authproxy.*"} |= "failed to load full request log from blob storage" | capture_state="unavailable"
```

## Local development

The Compose stack publishes the SeaweedFS S3 endpoint at
`http://localhost:9000`. It does not publish an administrative console.

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

## Hosted demo

The persistent demo runs SeaweedFS as the `demo-seaweedfs` Deployment in the
`demo` namespace. It is not exposed outside the cluster, so you need `kubectl`
access to the EKS cluster (see the
[EKS runbook](/deployment/eks-runbook/)) to browse it.

### With the AWS CLI

Forward the S3 port and read the generated credentials from the
`demo-seaweedfs-creds` Secret:

```bash
kubectl -n demo port-forward svc/demo-seaweedfs 8333:8333
```

In another terminal:

```bash
export AWS_ACCESS_KEY_ID=authproxy
export AWS_SECRET_ACCESS_KEY="$(kubectl -n demo get secret demo-seaweedfs-creds -o jsonpath='{.data.secret-key}' | base64 -d)"
export AWS_DEFAULT_REGION=us-east-1
aws --endpoint-url http://localhost:8333 s3 ls s3://authproxy-request-logs/
aws --endpoint-url http://localhost:8333 s3 ls s3://authproxy-request-logs/root.demo.demo-user/
```

These credentials grant full read/write access to the demo bucket. Avoid
`s3 rm`, and don't copy the secret key outside your shell session.

### With the filer web UI

The SeaweedFS filer serves a directory-listing UI on port 8888. It is
unauthenticated, so it is reachable only through a port-forward:

```bash
kubectl -n demo port-forward svc/demo-seaweedfs 8888:8888
```

Then open
[http://localhost:8888/buckets/authproxy-request-logs/](http://localhost:8888/buckets/authproxy-request-logs/).
The demo disables the UI's recursive-delete button.

### Storage capacity

SeaweedFS packs objects into 128 MB volume files on a 10 Gi `gp3` volume, so
many small request logs don't exhaust the filesystem's inodes. Check how much
of the volume is used with:

```bash
kubectl -n demo exec deploy/demo-seaweedfs -- df -h /data
```

## Existing MinIO data

SeaweedFS can't read MinIO's data directory, and nothing was migrated:

- **Local development:** SeaweedFS uses a new `seaweedfs_data` volume. The
  previous `minio_data` volume is left as it was. If you need old recorded
  blobs, export them through the old server's S3 API and import them into
  SeaweedFS before removing the old volume. Stop the old MinIO service before
  starting SeaweedFS on the same port.
- **Hosted demo:** request logs recorded before the switch to SeaweedFS were
  discarded. The `Deploy Demo` workflow deletes the old `demo-minio`
  Deployment, its `demo-minio-data` volume, and the `demo-minio-creds` Secret.
  Request details for those older requests are no longer available in the
  Admin UI.
