# SHBC × BuildOS — demo container
#
# Static-site image: nginx-alpine serves the contents of forks/smallhousingbc/demo
# on $PORT (Railway sets this; defaults to 8080 locally).
#
# Build context: the REPO ROOT (not this directory). Railway's
# RAILWAY_DOCKERFILE_PATH env var (or the build.dockerfilePath setting in
# railway.json) points here. The repo root is needed because Railway's
# clone gives us the whole monorepo; the COPY below scopes us to the demo.

FROM nginx:1.27-alpine

# Static assets
COPY forks/smallhousingbc/demo /usr/share/nginx/html

# nginx config — listens on $PORT, no caching for HTML so updates land
# immediately, long-cache for fingerprinted assets (we don't fingerprint
# yet, so leave the safe defaults).
COPY forks/smallhousingbc/nginx.conf /etc/nginx/templates/default.conf.template

# Railway provides $PORT at runtime. nginx:1.27-alpine's entrypoint
# expands ${PORT} via envsubst because we placed the config in
# /etc/nginx/templates/.
ENV PORT=8080
EXPOSE 8080

# Healthcheck: serve / and expect 200 with index.html.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- --spider http://localhost:${PORT:-8080}/ || exit 1
