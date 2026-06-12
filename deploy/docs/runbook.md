# AuthProxy EKS Runbook

Operational guide for the AuthProxy deployment cluster — what to do, in
what order, and what to check when things go sideways.

This file is the **what to do**. The **how it works** lives next to the
code:

| Topic | Reference |
|---|---|
| AWS infrastructure (VPC, EKS, IAM, Route53) | [`../terraform/README.md`](../terraform/README.md) |
| Terraform state (S3 + DynamoDB) | [`../terraform/bootstrap/README.md`](../terraform/bootstrap/README.md) |
| In-cluster components (ingress-nginx, cert-manager, external-dns, metrics-server) | [`../charts/bootstrap/README.md`](../charts/bootstrap/README.md) |
| AuthProxy application chart | [`../charts/authproxy/README.md`](../charts/authproxy/README.md) |

---

## Contents

1. [One-time setup](#1-one-time-setup) — bringing a cluster up from zero
2. [Granting kubectl access](#2-granting-kubectl-access)
3. [Rotating credentials](#3-rotating-credentials)
4. [Recovering from stuck deploys](#4-recovering-from-stuck-deploys)
5. [Cost monitoring](#5-cost-monitoring)
6. [Tear-down](#6-tear-down)

---

## 1. One-time setup

> **AWS account choice matters.** Be very sure `aws sts get-caller-identity`
> returns the right account before the first `terraform apply`. Tearing
> down a misplaced cluster is ~$3/day of wasted EKS control plane while
> you fix it (we have done this — see the postmortem in [section 4](#4-recovering-from-stuck-deploys)).

The applies are ordered. Don't reorder them.

### 1.1 Bootstrap Terraform state

```bash
cd deploy/terraform/bootstrap
terraform init
terraform apply        # local state, creates S3 bucket + DynamoDB lock table
terraform output       # save state_bucket_name for the next step
```

### 1.2 Wire the EKS module's backend

Edit `deploy/terraform/eks/backend.tf`, replace
`REPLACE_ME_FROM_BOOTSTRAP_OUTPUT` with the `state_bucket_name` output.
**Commit this change** — it's account-specific but not sensitive.

### 1.3 Apply the EKS module

```bash
cd ../eks
terraform init
terraform apply        # ~15-20 min (EKS control plane + node group rollout)
```

Save the outputs you'll need:

```bash
terraform output gh_actions_role_arn
terraform output external_dns_role_arn
terraform output route53_zone_id
terraform output domain_name
terraform output route53_name_servers
```

### 1.4 Delegate the domain at the registrar

**Common pitfall:** Route53 auto-creates a hosted zone at registration
time. Terraform creates a *second* zone for the same domain. The
registrar's NS list may still point at the auto-created zone (or, if you
previously had a different cluster, the *deleted* zone). Either case
breaks DNS in a way that's hard to debug.

Always update the registrar's NS list explicitly:

```bash
NS_VALUES=$(terraform output -raw route53_name_servers \
  | tr -d '[]"' | tr ',' '\n' | xargs -n1 | head -4)

# AWS Console: Route 53 → Registered domains → <domain> → "Add or edit name servers"
# Or via CLI (note: route53domains lives only in us-east-1):
aws route53domains update-domain-nameservers \
  --region us-east-1 \
  --domain-name <your-domain> \
  --nameservers \
    Name=<ns-1> Name=<ns-2> Name=<ns-3> Name=<ns-4>
```

Verify with `dig +short NS <your-domain> @8.8.8.8` — should return the
four AWS NS hostnames within ~10-60 min.

### 1.5 Wire kubectl locally

```bash
aws eks update-kubeconfig --name authproxy-eks --region us-east-1
kubectl get nodes      # smoke test
```

### 1.6 Wire GitHub Actions OIDC

```bash
gh secret set AWS_ROLE_ARN --body "$(terraform output -raw gh_actions_role_arn)"
```

Then in the repo settings → Environments → create a new environment
named **`gha-eks`** (matches the trust-policy condition in
`deploy/terraform/eks/iam_oidc.tf`). No required reviewers needed
initially; add them once production workflows live there.

### 1.7 Verify the OIDC chain end-to-end

```bash
gh workflow run "Verify EKS OIDC"
gh run watch
```

Green run = federated trust policy is working. (For deeper background
on what OIDC is doing here, see the explanation in `deploy/terraform/eks/iam_oidc.tf`.)

### 1.8 Install the in-cluster bootstrap layer

```bash
cd deploy/charts/bootstrap
helm dependency update

helm upgrade --install authproxy-bootstrap . \
  --namespace kube-system \
  --create-namespace \
  --wait --timeout 5m \
  --set "global.acmeEmail=$(git config user.email)" \
  --set "global.hostedZoneId=$(cd ../../terraform/eks && terraform output -raw route53_zone_id)" \
  --set "global.domain=$(cd ../../terraform/eks && terraform output -raw domain_name)" \
  --set "external-dns.serviceAccount.annotations.eks\.amazonaws\.com/role-arn=$(cd ../../terraform/eks && terraform output -raw external_dns_role_arn)" \
  --set "external-dns.domainFilters[0]=$(cd ../../terraform/eks && terraform output -raw domain_name)" \
  --set "external-dns.zoneIdFilters[0]=$(cd ../../terraform/eks && terraform output -raw route53_zone_id)"
```

`--wait` is load-bearing — without it, cert-manager's webhook race
trips the `ClusterIssuer` post-install hook. See
[section 4.1](#41-cert-manager-webhook-race) if it surfaces.

### 1.9 Smoke test

```bash
# Replace YOUR_DOMAIN in the example, then apply:
sed "s/YOUR_DOMAIN/$(cd ../../terraform/eks && terraform output -raw domain_name)/g" \
  examples/hello-echo.yaml | kubectl apply -f -

# DNS landing:
kubectl -n hello-echo get ingress,certificate -w   # certificate Ready ≈ 3 min

# End-to-end:
curl -fsSI https://hello.$(cd ../../terraform/eks && terraform output -raw domain_name)/

# Tear down once green:
kubectl delete ns hello-echo
```

---

### 1.10 Wire the auto-deploy workflow

The `Deploy Demo` workflow (`.github/workflows/deploy-demo.yml`)
applies the Kustomize demo overlay on every push to `main` with the
image tag pinned to that commit's `sha-<short>`. Demo data is seeded
separately through the manual `Seed Demo` workflow, so application
rollouts and catalog refreshes can be run independently. One-time setup:

1. **Create the demo namespace + keypair Secrets** (these are
   operator-provided, not auto-generated — see
   [`deploy/kustomize/authproxy-demo/README.md`](../kustomize/authproxy-demo/README.md)
   for the required secret names). The workflow assumes
   `demo-jwt`, `demo-encryption`, `demo-demo-shell-key`, and
   `demo-actors` already exist in the `demo` namespace. Persistent
   backing-store Secrets (`demo-db`, `demo-redis-creds`, and
   `demo-minio-creds`) are also preserved across Kustomize applies.

2. **Set repo Variables** (Settings → Variables → Actions):

   | Variable        | Example          |
   |-----------------|------------------|
   | `AWS_REGION`    | `us-east-1`      |
   | `EKS_CLUSTER`   | `authproxy-eks`  |
   | `DEMO_HOSTNAME` | `demo.authproxy.net` |

   (The Let's Encrypt email is configured on the bootstrap chart's
   ClusterIssuer at install time — see Section 1.8 — and isn't
   re-passed per deploy.)

   `AWS_ROLE_ARN` should already be set as a Secret from Section 1.6.

3. **Confirm the `gha-eks` environment** (from Section 1.6) exists.
   No required reviewers = fully automatic deploys; add reviewers in
   repo settings to gate deploys on approval without changing the
   workflow.

4. (Optional) **Trigger the first run manually** before the next
   merge to confirm wiring:

   ```bash
   gh workflow run "Deploy Demo"
   gh run watch
   ```

   Once green, every subsequent merge to `main` deploys automatically.

5. **Seed or refresh demo catalog data manually** when needed:

   ```bash
   gh workflow run "Seed Demo" --ref main
   gh run watch
   ```

   The seed workflow renders `deploy/kustomize/authproxy-demo/overlays/demo/seed`
   and runs a one-shot Kubernetes Job against the already-deployed demo
   services.

### 1.11 PR dev demo environments

The `Deploy Dev` workflow (`.github/workflows/deploy-dev.yml`) creates
an isolated `authproxy-demo` environment for same-repository pull
requests that carry the `deploy:demo` label. Add the label when opening
the PR, or add it to an existing PR later; either path triggers the dev
deployment. Subsequent pushes to a labeled PR redeploy the same
environment with the `pr-<number>` image tag.

The `Build Image` workflow always runs a simple PR Docker build for the
main `authproxy` image. It only publishes the PR image tag and builds the
demo-only `authproxy-demo-shell` / `authproxy-demo-seed` images when the
PR carries `deploy:demo`; merges and release tags still publish the full
image set.

The `Teardown Dev` workflow still runs on PR close regardless of labels,
so merged or closed PRs tear down any previously-created dev demo
environment even if the opt-in label has since been removed.

#### 1.11.1 Dev storage profile

Per-branch dev environments intentionally use a smaller, disposable
storage profile than the persistent demo environment. `Deploy Dev`
applies `deploy/kustomize/authproxy-demo/overlays/dev`; the regular
`Deploy Demo` workflow applies `deploy/kustomize/authproxy-demo/overlays/demo`.

| Environment | Main DB | Session/task/rate-limit store | Blob storage | Backing workloads |
|---|---|---|---|---|
| `demo.authproxy.net` | Postgres | Redis | MinIO | AuthProxy, demo shell, go-oauth2-server, Postgres, Redis, MinIO, optional Grafana, and manual seed Jobs |
| per-branch dev | SQLite under `/tmp` in the AuthProxy pod | in-process `miniredis` | filesystem under `/tmp/authproxy-blobs` in the AuthProxy pod | AuthProxy, demo shell, seed Job, go-oauth2-server, and optional Grafana |

This cuts the required stateful backing containers from three to zero
for PR environments: no Postgres, Redis, or MinIO pods are rendered. The
tradeoff is durability. A pod restart, reschedule, or redeploy can lose
SQLite data, miniredis state, queue/session/OAuth round-trip state, and
filesystem blobs. That is acceptable for PR demos because the next
deploy reruns the seed Job and recreates the catalog and demo actors.

Do not use the dev overlay for `demo.authproxy.net`, customer installs,
or any environment where users expect stored connections, sessions,
queues, request logs, or metrics to survive pod replacement.

To verify the rendered profile before deploying:

```bash
kubectl kustomize deploy/kustomize/authproxy-demo/overlays/dev \
  >/tmp/authproxy-dev.yaml

rg -n "provider: (sqlite|miniredis|filesystem)|/tmp/authproxy-blobs|kind: (Deployment|Job|StatefulSet)|name: dev-(postgresql|redis|minio)" \
  /tmp/authproxy-dev.yaml
```

Expected result: the AuthProxy config contains `provider: sqlite`,
`provider: miniredis`, and `provider: filesystem`; the AuthProxy pod
mounts `/tmp/authproxy-blobs`; there are no `dev-postgresql`,
`dev-redis`, `dev-minio`, or `StatefulSet` matches.

To verify a live PR namespace:

```bash
NS=dev-my-feature-branch

kubectl -n "$NS" get deploy,job,statefulset,pod
kubectl -n "$NS" get pod -l app.kubernetes.io/name=postgresql
kubectl -n "$NS" get pod -l app.kubernetes.io/name=redis
kubectl -n "$NS" get pod -l app.kubernetes.io/name=minio
kubectl -n "$NS" get configmap -o yaml | rg -n "provider: (sqlite|miniredis|filesystem)|/tmp/authproxy-blobs"
```

The dependency pod queries should return no resources. The rendered
ConfigMap should show the slim storage providers.

Rollback option: rerun the workflow after reverting the Kustomize
overlay change. Disposable data in the old AuthProxy pod should be
treated as lost.

## 2. Granting kubectl access

The cluster uses the **modern EKS access-entry model**
(`authentication_mode: API` in `deploy/terraform/eks/cluster.tf`). No
`aws-auth` ConfigMap.

### 2.1 Grant access to an IAM user or role

Add an entry to `deploy/terraform/eks/cluster.tf` next to the existing
`aws_eks_access_entry "gh_actions"` resource:

```hcl
resource "aws_eks_access_entry" "alice" {
  cluster_name  = module.eks.cluster_name
  principal_arn = "arn:aws:iam::<account-id>:user/alice"   # or :role/<role-name>
  type          = "STANDARD"
}

resource "aws_eks_access_policy_association" "alice_admin" {
  cluster_name  = module.eks.cluster_name
  principal_arn = aws_eks_access_entry.alice.principal_arn
  # AmazonEKSAdminPolicy   = namespace-scoped admin (preferred for engineers)
  # AmazonEKSClusterAdminPolicy = cluster-wide admin (reserve for ops + CI)
  policy_arn    = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSAdminPolicy"

  access_scope {
    type       = "namespace"
    namespaces = ["default", "hello-echo"]   # narrow as appropriate
  }
}
```

Then `terraform apply` in `deploy/terraform/eks/`. The engineer can run:

```bash
aws eks update-kubeconfig --name authproxy-eks --region us-east-1
kubectl auth can-i get pods -n default
```

### 2.2 Granting access without a Terraform change (urgent only)

```bash
aws eks create-access-entry \
  --cluster-name authproxy-eks \
  --principal-arn arn:aws:iam::<account-id>:user/<name> \
  --type STANDARD

aws eks associate-access-policy \
  --cluster-name authproxy-eks \
  --principal-arn arn:aws:iam::<account-id>:user/<name> \
  --policy-arn arn:aws:eks::aws:cluster-access-policy/AmazonEKSAdminPolicy \
  --access-scope type=cluster
```

**Backfill the Terraform** within the same day — out-of-band access
entries drift from state and break the next apply.

### 2.3 Revoking access

```bash
aws eks delete-access-entry \
  --cluster-name authproxy-eks \
  --principal-arn arn:aws:iam::<account-id>:user/<name>
```

(Or remove the resource block in Terraform + apply.)

---

## 3. Rotating credentials

### 3.1 Rotating the GitHub Actions OIDC role

The role itself doesn't have credentials to rotate — it's federated. To
rotate its *permissions*:

```bash
# Edit deploy/terraform/eks/iam_oidc.tf — tighten policy or trust scope
terraform apply
```

To rotate the *trust* (e.g. revoke a compromised repo): change the
`condition.values[*]` `repo:rmorlok/authproxy:...` subjects to a
narrower set, apply.

### 3.2 Rotating the JWT signing keypair

The chart references a Secret (`jwt.existingSecret`) — the chart never
generates the keys. To rotate:

```bash
# 1. Generate the new keypair locally.
workdir=$(mktemp -d)
openssl genrsa -out "$workdir/system" 2048
openssl rsa -in "$workdir/system" -pubout -out "$workdir/system.pub"

# 2. Re-create the Secret (kubectl create --dry-run + apply pattern lets
# you replace the data without losing the resource).
kubectl create secret generic authproxy-jwt-new \
  --from-file=system="$workdir/system" \
  --from-file=system.pub="$workdir/system.pub" \
  --namespace authproxy \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Update the chart values to point at the new Secret + helm upgrade.
helm upgrade authproxy <chart> --reuse-values \
  --set jwt.existingSecret=authproxy-jwt-new

# 4. Once rollout completes and existing sessions have aged out, delete
# the old Secret.
kubectl -n authproxy delete secret authproxy-jwt
```

In-flight sessions signed by the old key are invalidated at rollout
unless the AuthProxy server is configured for dual-key validation
(currently it isn't — JWT signing key swap is an effective sign-out for
all users).

### 3.3 Rotating the global AES key

The encryption key wraps every credential in the database. Rotation is
**not** a hot swap — the re-encryption registry (see
`internal/database/reencrypt_registry.go`) walks every encrypted column
and rewraps. Procedure:

```bash
# 1. Generate the new key locally:
openssl rand 32 > /tmp/global_aes.key.new

# 2. Add it as a SECOND key in the encryption Secret. The chart only
#    surfaces a single mountpoint right now; you'll need to add a
#    sibling secret + values override. (See open issue: TBD — the chart
#    doesn't yet support overlapping keys.)

# 3. Trigger the re-encryption job via the admin API:
ap encfield reencrypt --new-key /etc/authproxy/keys/encryption/global_aes.key.new

# 4. Once the registry reports all columns rewrapped, swap the keys:
mv /tmp/global_aes.key.new /tmp/global_aes.key
helm upgrade authproxy <chart> --reuse-values
```

If you're sitting at the "I can't rotate because there's no dual-key
window" problem, file an issue against `internal/encrypt/` rather than
hacking around it.

### 3.4 Rotating the database password

Postgres only. SQLite has no auth.

```bash
# 1. Rotate the password at the database (RDS console or `ALTER USER`):
psql -h <host> -U authproxy -c "ALTER USER authproxy WITH PASSWORD '<new>';"

# 2. Update the Secret the chart references:
kubectl -n authproxy create secret generic authproxy-db \
  --from-literal=AUTHPROXY_DB_PASSWORD='<new>' \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Restart AuthProxy to pick up the new password:
kubectl -n authproxy rollout restart deployment authproxy
```

The chart wires the password via `envFrom` (see
`deploy/charts/authproxy/templates/deployment.yaml`); existing
connections drain on rollout.

### 3.5 Rotating Let's Encrypt's ACME account key

The `ClusterIssuer` post-install hook creates a Secret named
`<issuer-name>-account-key`. To rotate:

```bash
kubectl -n cert-manager delete secret letsencrypt-prod-account-key
helm upgrade authproxy-bootstrap deploy/charts/bootstrap \
  --reuse-values --namespace kube-system
```

cert-manager re-registers with Let's Encrypt under a new key on the
next reconcile.

---

## 4. Recovering from stuck deploys

### 4.1 cert-manager webhook race

**Symptom:**

```
Error: failed post-install: Hook post-install ClusterIssuer failed:
  no endpoints available for service "...-cert-manager-webhook"
```

**Cause:** the ClusterIssuer post-install hook fired before the
cert-manager webhook's pod was Ready. The chart pins our hook weight
above cert-manager's `startupapicheck` to avoid this, but only when
`--wait` is also set on the helm install (otherwise nothing makes the
main release block on cert-manager being Ready).

**Fix:** add `--wait --timeout 5m` to the install command and re-run:

```bash
helm upgrade --install authproxy-bootstrap . --wait --timeout 5m [...]
```

If you can't wait, apply the ClusterIssuer manually as a one-shot:

```bash
kubectl apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata: { name: letsencrypt-prod }
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: <your-email>
    privateKeySecretRef: { name: letsencrypt-prod-account-key }
    solvers:
      - http01: { ingress: { ingressClassName: nginx } }
EOF
```

### 4.2 external-dns + multiple Route53 zones for the same domain

**Symptom:** external-dns logs `Desired change: CREATE hello.<domain> A`
repeatedly, followed by `AccessDenied: ... is not authorized to perform:
route53:ChangeResourceRecordSets on resource: arn:aws:route53:::hostedzone/<id>`.
A record never appears.

**Cause:** AWS auto-creates a hosted zone when you register a domain.
Terraform creates a *second* zone for the same domain. external-dns
sees both via `domainFilters`, tries to write to both, gets denied on
the registrar-auto-created one (IAM role only authorizes the Terraform
zone), and a single AccessDenied marks the whole reconcile failed.

**Fix:** scope external-dns to one zone explicitly:

```bash
helm upgrade authproxy-bootstrap deploy/charts/bootstrap --reuse-values \
  --set "external-dns.zoneIdFilters[0]=$(cd deploy/terraform/eks && terraform output -raw route53_zone_id)"
```

Then **delete the orphan zone** (it has only NS+SOA records):

```bash
aws route53 list-hosted-zones --query "HostedZones[?Name == 'authproxy.net.']"
# Identify the registrar-created zone (CallerReference will mention "Route53 Registrar")
aws route53 delete-hosted-zone --id <orphan-zone-id>
```

### 4.3 Stuck ACME order / `InvalidChangeBatch: TXT record already exists`

**Symptom:** Certificate stays `Ready=False` for > 10 min;
external-dns logs `Tried to create resource record set ... but it already exists`.

**Cause:** external-dns half-wrote records on an earlier failed
reconcile. external-dns uses `CREATE` (not `UPSERT`), so it fails on
the existing TXT record forever.

**Fix:** delete the stale TXT record manually so external-dns can
re-CREATE the full set:

```bash
ZONE_ID=$(cd deploy/terraform/eks && terraform output -raw route53_zone_id)
aws route53 list-resource-record-sets --hosted-zone-id "$ZONE_ID" \
  --query "ResourceRecordSets[?starts_with(Name, '<the-host>.')]" --output table
# Find the stale TXT, then:
aws route53 change-resource-record-sets --hosted-zone-id "$ZONE_ID" \
  --change-batch '{"Changes":[{"Action":"DELETE","ResourceRecordSet":{...}}]}'
```

If the ACME order itself is stuck because Let's Encrypt cached the
NXDOMAIN before DNS came up, delete the Order to force a fresh round:

```bash
kubectl -n <ns> delete order <order-name>
# cert-manager creates a fresh order automatically.
```

### 4.4 Helm release stuck in `failed` state

**Symptom:** `helm list` shows `STATUS=failed`; `helm upgrade --install`
errors `cannot patch ... has been modified` or `another operation in progress`.

**Fix in increasing severity:**

```bash
# A. Just retry — most "failed" states resolve on next upgrade.
helm upgrade --install authproxy-bootstrap . --wait [...]

# B. Roll back to the last successful revision:
helm history authproxy-bootstrap -n kube-system
helm rollback authproxy-bootstrap <revision> -n kube-system

# C. Nuclear: uninstall + reinstall (loses any in-cluster state the
#    release owned that isn't backed by a PVC):
helm uninstall authproxy-bootstrap -n kube-system
helm install authproxy-bootstrap . --namespace kube-system --wait [...]
```

CRDs survive `helm uninstall` of cert-manager because the chart sets
`crds.keep=true`. Workload Certificates / Issuers in other namespaces
survive uninstall and the next install will adopt them.

### 4.5 Cluster destroyed in the wrong AWS account (or registrar pointed at a deleted zone)

We have done this. The recovery procedure lives in
[`deploy/terraform/README.md`](../terraform/README.md) under
"Destroying" + "Apply (first time)". TL;DR:

1. `terraform destroy` in `deploy/terraform/eks/` (against the wrong account's creds).
2. Empty the state bucket, bypass `prevent_destroy`, `terraform destroy` in `bootstrap/`.
3. `gh secret delete AWS_ROLE_ARN` so the next install doesn't reference the deleted role.
4. At the registrar (likely Route53 too), revert NS records or hold them — they're pointing at a deleted zone right now.
5. Switch AWS credentials, re-run the full Section 1 against the right account.
6. **Update the registrar's NS list to the new zone's NS** — the new Route53 zone gets a fresh set, the registrar still has the old (deleted) set.

### 4.6 Stranded AWS resources after `terraform destroy`

After destroying the cluster, occasionally an ELB / target group / ENI
hangs around because something in the cluster owned it via annotation
rather than Terraform. Find them with:

```bash
# Load balancers in the cluster's VPC:
aws elbv2 describe-load-balancers \
  --query "LoadBalancers[?VpcId=='$VPC_ID'].LoadBalancerArn"

# Network interfaces still attached to deleted subnets:
aws ec2 describe-network-interfaces \
  --filters "Name=vpc-id,Values=$VPC_ID" \
  --query "NetworkInterfaces[].NetworkInterfaceId"
```

Delete them with `aws elbv2 delete-load-balancer` /
`aws ec2 delete-network-interface`, then rerun `terraform destroy`.

---

## 5. Cost monitoring

### 5.1 Steady-state target

~$150-165/mo with the defaults in `deploy/terraform/eks/variables.tf`:

| Item                         | /mo  |
|------------------------------|------|
| EKS control plane            | $73  |
| 2× t3.medium (on-demand)     | $60  |
| Single NAT gateway           | $32  |
| Route53 hosted zone          | $0.50|
| S3 + DynamoDB (state)        | <$1  |

### 5.2 Setting a budget alert

```bash
aws budgets create-budget --account-id $(aws sts get-caller-identity --query Account --output text) \
  --budget '{
    "BudgetName": "authproxy-monthly",
    "BudgetLimit": {"Amount": "200", "Unit": "USD"},
    "TimeUnit": "MONTHLY",
    "BudgetType": "COST"
  }' \
  --notifications-with-subscribers '[{
    "Notification": {"ComparisonOperator": "GREATER_THAN", "NotificationType": "ACTUAL", "Threshold": 80, "ThresholdType": "PERCENTAGE"},
    "Subscribers": [{"Address": "you@example.com", "SubscriptionType": "EMAIL"}]
  }]'
```

(Or via Console: Billing → Budgets → Create budget.)

### 5.3 Common cost spikes to watch for

| Symptom                          | Likely cause                                                          |
|----------------------------------|-----------------------------------------------------------------------|
| Sudden +$30/mo                   | Multi-AZ NAT enabled (single → three NATs at $32 each)                |
| Sudden +$60/mo                   | Node group desired_size bumped without thinking                       |
| Sudden +$0.025/hr per ELB        | Per-branch dev env left running (each gets its own NLB)               |
| Sustained higher data-out        | A workload pulling from external APIs over NAT — move to VPC endpoint |

A weekly look at Cost Explorer → "Group by Service" catches most.

---

## 6. Tear-down

```bash
# 1. Drain anything that owns AWS resources outside Terraform's view:
helm uninstall authproxy-bootstrap -n kube-system   # releases the NLB
kubectl get ns -o name | xargs -L1 kubectl delete ns --wait=false  # PVCs / LB Services in workload nses
# Wait until `aws elbv2 describe-load-balancers --query "LoadBalancers[?VpcId=='$VPC_ID']"` is empty.

# 2. Destroy the EKS module:
cd deploy/terraform/eks
terraform destroy

# 3. (Optional) Destroy the state bucket too. See bootstrap/README — the
#    bucket has `prevent_destroy = true` and needs deliberate steps.

# 4. Revert the registrar's NS records — they're pointing at a now-deleted
#    zone. Either delegate to the next hosting provider or set to the
#    registrar's defaults.
```

A clean tear-down leaves the AWS account back at zero spend for this
project. A *messy* tear-down leaves orphan ELBs / volumes that keep
billing — see [section 4.6](#46-stranded-aws-resources-after-terraform-destroy).
