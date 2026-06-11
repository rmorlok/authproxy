# AuthProxy Demo Kustomize Manifests

This tree is the Kustomize replacement scaffold for the demo/dev deployment
path. The customer-facing Helm chart remains in `deploy/charts/authproxy`;
these manifests are only for hosted demo environments.

The base contains the shared AuthProxy, demo-shell, and go-oauth2-server
workloads. Overlays choose backing services and public hostnames:

- `overlays/demo` targets `demo.authproxy.net` and includes Postgres, Redis,
  and MinIO backing workloads.
- `overlays/dev` targets an example per-branch namespace and keeps the slim
  dev profile: SQLite, embedded miniredis, and filesystem blob storage.

Render locally:

```bash
kubectl kustomize deploy/kustomize/authproxy-demo/overlays/demo
kubectl kustomize deploy/kustomize/authproxy-demo/overlays/dev
```

`Deploy Demo` renders `overlays/demo`, rewrites the checked-out overlay with
the selected image tag and configured hostname, and applies the resulting
manifest with `kubectl apply`. The demo overlay includes Grafana at
`https://<hostname>/grafana` with the AuthProxy datasource and sample app
metrics dashboard provisioned from Kustomize ConfigMaps.

During the Helm-to-Kustomize cutover, `Deploy Demo` preserves the operator
Secrets listed below, uninstalls any legacy `demo` / `authproxy-demo` Helm
release in the `demo` namespace, reapplies the preserved Secrets, and removes
old Helm-managed Deployments, StatefulSets, and Ingresses that can conflict with
Kustomize's selectors and host rules. Later deploys are idempotent because the
legacy Helm releases and Helm-managed resources are absent.

Secrets are still created by workflow/setup steps. The overlays expect names
that match the existing release convention after `namePrefix` is applied:

- `<env>-jwt`
- `<env>-encryption`
- `<env>-actors`
- `<env>-demo-shell-key`
- `<env>-db` and `<env>-redis-creds` for the demo overlay

Seeding is intentionally not part of the Kustomize deployment apply. Run the
`Seed Demo` GitHub Actions workflow to reseed the persistent demo environment
on demand; it renders `overlays/demo/seed` and applies the resulting Job.
Dev seeding will be run by the per-branch deploy workflow after the environment
is applied.
