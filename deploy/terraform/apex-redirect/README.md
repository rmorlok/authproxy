# AuthProxy apex redirect

This stack redirects HTTP and HTTPS `GET` and `HEAD` requests on `authproxy.net`
to the docs homepage with a temporary `302`. It uses the existing public Route
53 zone and stores state at `apex-redirect/terraform.tfstate` in the shared
state bucket.

## Deploy

Run from this directory with AWS credentials for the AuthProxy account. On the
first apply, provision the certificate and distribution before publishing the
apex aliases.

```sh
terraform init
terraform plan -var=publish_dns=false -out=redirect.tfplan
terraform apply redirect.tfplan
```

Verify the apex redirect and TLS certificate against the deployed distribution.
Both requests should print `302 https://docs.authproxy.net/`.

```sh
distribution=$(terraform output -raw cloudfront_domain_name)
curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  --connect-to "authproxy.net:80:$distribution:80" http://authproxy.net/
curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  --connect-to "authproxy.net:443:$distribution:443" https://authproxy.net/
```

Publish the `A` and `AAAA` aliases, then repeat both requests using public DNS.
The follow-up requests should each reach the docs in one hop and return `200`.

```sh
terraform plan -out=redirect-dns.tfplan
terraform apply redirect-dns.tfplan

for scheme in http https; do
  curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' \
    "$scheme://authproxy.net/"
  curl -sS -o /dev/null -L --max-redirs 1 \
    -w '%{http_code} %{num_redirects} %{url_effective}\n' "$scheme://authproxy.net/"
done
```

For a DNS rollback, plan and apply with `-var=publish_dns=false` and confirm
that the plan removes only the two apex aliases. Keep the ACM validation record
while the distribution uses that certificate. When the website is ready, replace
the redirect on this distribution or move the apex to the new hosting before
retiring the stack.
