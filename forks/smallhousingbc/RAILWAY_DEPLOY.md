# Deploying to Railway — `smallhousing.futurebuild.ai`

Step-by-step to wire this branch up to a Railway service tied to the
`smallhousing-demo` branch with the `smallhousing.futurebuild.ai`
custom domain. ~5 minutes UI work + DNS propagation.

The repo already has everything Railway needs:

- [`railway.json`](../../railway.json) at the repo root → tells Railway
  which Dockerfile to build (`forks/smallhousingbc/Dockerfile`).
- [`forks/smallhousingbc/Dockerfile`](Dockerfile) → builds a small
  `nginx:1.27-alpine` image with the static demo + the nginx config.
- [`forks/smallhousingbc/nginx.conf`](nginx.conf) → listens on `$PORT`
  (Railway sets this), gzip + cache + light security headers, hash
  routing handled client-side.

## 1. Connect the repo + branch

1. Go to **<https://railway.com/new>** → **Deploy from GitHub repo**
2. Pick **`futurebuildai/buildos`**
3. **Settings → Source → Branch** → set to **`smallhousing-demo`**
4. **Settings → Source → Watch Paths** → leave empty (every push to
   the branch should rebuild)
5. Trigger the first deploy (Settings → Deployments → Redeploy, or
   just push a no-op commit)

Railway picks up `railway.json` automatically. Build time: ~30–45s.

## 2. Verify the service is healthy

Once the deploy turns green:

```
curl -sS https://<railway-generated>.up.railway.app/ -o /dev/null -w "%{http_code}\n"
# expect: 200
curl -sS https://<railway-generated>.up.railway.app/data/projects.json | head -c 80
# expect: { "projects": [ {…
```

## 3. Custom domain — `smallhousing.futurebuild.ai`

In the Railway service:

1. **Settings → Networking → Custom Domain** → **+ Custom Domain**
2. Enter **`smallhousing.futurebuild.ai`**
3. Railway shows a target like `xyzabc.up.railway.app` (or a CNAME
   pointer)

In your DNS provider for `futurebuild.ai`:

```
Type   Name          Value
CNAME  smallhousing  xyzabc.up.railway.app
TTL    3600 (or 300 while testing)
```

Once the DNS propagates (usually < 5 min), Railway issues the cert
automatically. The domain shows ✅ in the Networking panel.

## 4. (Optional) Auto-deploy on push

Railway watches the branch by default once connected. Every push to
`smallhousing-demo` triggers a fresh build. If you want to merge
`main` into this branch routinely (e.g., to keep the discovery docs
fresh), the deploy will only rebuild if files inside the build context
change — but the entire Docker layer is small (~10 MB), so the
rebuild is cheap regardless.

## 5. Updating the demo

```
git checkout smallhousing-demo
# edit files under forks/smallhousingbc/demo/
git add forks/smallhousingbc/demo
git commit -m "demo: tweak feed copy"
git push origin smallhousing-demo
```

Railway picks it up automatically.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Build fails at `COPY forks/smallhousingbc/demo` | Branch isn't `smallhousing-demo` (the demo files don't exist on `main`) — check Settings → Source → Branch. |
| 502 on every request | nginx didn't start. Check Logs tab — the `envsubst` template in `default.conf.template` requires `$PORT` to be set; Railway always sets it, but local-only Docker users must `-e PORT=8080`. |
| `smallhousing.futurebuild.ai` shows "domain not configured" | DNS hasn't propagated yet, or the CNAME points at the wrong subdomain. `dig smallhousing.futurebuild.ai` should show the Railway target. |
| Stale demo data | nginx caches HTML for `no-store`, JSON for 60s, assets for 1h. Hard-refresh (Cmd-Shift-R) bypasses the JSON cache. |

## Cost

The demo is a static site behind nginx. On Railway's hobby plan it
runs at the lowest tier (~$1–3/month with cold-start + minimal
traffic). Production demo to a VC / partner: pre-warm via cron pinger
or upgrade to "always on" if it matters.

## What's NOT in this deploy

- No backend, no database, no auth — it's a UI demo.
- No CI/CD beyond Railway's built-in branch watcher.
- No analytics / observability — add Railway's logging or a Plausible
  snippet if you want page-view metrics.
- No content management — to update copy or add a project, edit the
  JSON files in `forks/smallhousingbc/demo/data/` and push.
