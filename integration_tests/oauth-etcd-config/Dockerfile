FROM gcr.io/etcd-development/etcd:v3.5.17 AS etcd
FROM alpine:3.20
RUN apk add --no-cache bash
COPY --from=etcd /usr/local/bin/etcdctl /usr/local/bin/etcdctl
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
