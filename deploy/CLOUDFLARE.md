# Cloudflare setup — octad.gg migration

Edge config for the `lioctad.org → octad.gg` migration. TLS terminates at
Cloudflare; the app is reached over a Cloudflare Access tunnel (see
`deploy/docker-compose.yaml`). Nothing here is read by the app — it's edge/DNS
state you apply once in Cloudflare.

Two zones are involved:
- **`octad.gg`** — the new live zone (tunnel + DNS).
- **`lioctad.org`** — kept alive **permanently** for the 301 redirect only.

---

## 1. octad.gg — tunnel + DNS

Use `deploy/cloudflared-config.yml`. `cloudflared tunnel route dns octad-gg
octad.gg` creates the proxied CNAME `octad.gg → <UUID>.cfargotunnel.com`
(orange-cloud). Cloudflare Universal SSL auto-provisions the edge cert for the
apex + `www`. The tunnel origin stays `http://localhost:4444` — unchanged from
today.

`status.octad.gg` is a separate service — route it in its own tunnel/DNS. There
is no `dev.` host and no `database.` host (the games DB is `octad.gg/db`).

---

## 2. lioctad.org → octad.gg — permanent 301 redirect

A **Single Redirect** (Rulesets `http_request_dynamic_redirect` phase) in the
**lioctad.org** zone. Preserves path + query, so
`lioctad.org/db?x=1 → https://octad.gg/db?x=1`.

For the edge to intercept requests, `lioctad.org` needs a **proxied** DNS record
(the redirect runs before any origin is contacted, so the target can be a dummy):

```
Type A   Name @     Content 192.0.2.1   Proxy: ON (orange cloud)
Type A   Name www   Content 192.0.2.1   Proxy: ON (orange cloud)
```

### Dashboard
Rules → **Redirect Rules** → Create → *Single Redirect*:
- **When incoming requests match:** `(http.host eq "lioctad.org") or (http.host eq "www.lioctad.org")`
- **Then… URL redirect → Dynamic**
  - Expression: `concat("https://octad.gg", http.request.uri.path)`
  - **Preserve query string:** ON
  - **Status code:** `301`

### API (Rulesets)
```bash
curl -X PUT \
  "https://api.cloudflare.com/client/v4/zones/${LIOCTAD_ZONE_ID}/rulesets/phases/http_request_dynamic_redirect/entrypoint" \
  -H "Authorization: Bearer ${CF_API_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "rules": [
      {
        "action": "redirect",
        "description": "Permanent redirect lioctad.org -> octad.gg",
        "expression": "(http.host eq \"lioctad.org\") or (http.host eq \"www.lioctad.org\")",
        "action_parameters": {
          "from_value": {
            "status_code": 301,
            "target_url": { "expression": "concat(\"https://octad.gg\", http.request.uri.path)" },
            "preserve_query_string": true
          }
        }
      }
    ]
  }'
```

### Terraform
```hcl
resource "cloudflare_ruleset" "lioctad_redirect" {
  zone_id = var.lioctad_zone_id
  name    = "lioctad.org -> octad.gg"
  kind    = "zone"
  phase   = "http_request_dynamic_redirect"

  rules {
    action      = "redirect"
    description = "Permanent redirect lioctad.org -> octad.gg"
    expression  = "(http.host eq \"lioctad.org\") or (http.host eq \"www.lioctad.org\")"
    action_parameters {
      from_value {
        status_code = 301
        target_url { expression = "concat(\"https://octad.gg\", http.request.uri.path)" }
        preserve_query_string = true
      }
    }
  }
}
```

---

## 3. (Optional) www.octad.gg → octad.gg canonical redirect

Same Single Redirect pattern, but in the **octad.gg** zone. If you add this, drop
the `www.octad.gg` ingress entry from `cloudflared-config.yml` (www never reaches
the tunnel):
- Match: `http.host eq "www.octad.gg"`
- Dynamic target: `concat("https://octad.gg", http.request.uri.path)`, preserve query, 301.

---

## Cutover ordering (matches the migration plan)

1. Bring up the octad.gg tunnel + DNS (step 1); test the app directly on octad.gg.
2. Deploy the app with `SITE_DOMAIN=octad.gg` **and** `EXTRA_ORIGINS=https://lioctad.org`
   (transition allowlist — see `docker-compose.yaml`).
3. **Immediately** enable the lioctad.org 301 (step 2).
4. Soak, then remove `EXTRA_ORIGINS` and redeploy.

Notes:
- Keep the lioctad.org zone on HTTPS forever so the redirect + its existing HSTS
  keep resolving. `lioctad.org` is **not** HSTS-preloaded, so the 301 is safe.
- The domain move invalidates all WebAuthn passkeys (RPID is host-bound) and
  resets all sessions (`sid` is host-only) — expected, announced in the news feed.
