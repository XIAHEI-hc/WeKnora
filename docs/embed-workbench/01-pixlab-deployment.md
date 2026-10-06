# PixLab native workbench deployment

The PixLab workbench is served from the same public origin as PixLab. PixLab's
gateway forwards only these two path prefixes to WeKnora:

- `/weknora-workbench/` to the WeKnora `frontend` service.
- `/api/v1/pixlab-workbench/` to the WeKnora `app` service.

The ordinary WeKnora `/api/v1/*` surface must not be exposed through the
PixLab gateway.

## Shared Docker network

PixLab creates the external Docker network `pixlab-internal`. Attach only the
two WeKnora services needed by the gateway by adding the integration overlay:

```bash
docker compose \
  -f docker-compose.yml \
  -f docker-compose.pixlab.yml \
  config

docker compose \
  -f docker-compose.yml \
  -f docker-compose.pixlab.yml \
  up -d app frontend
```

Set `PIXLAB_DOCKER_NETWORK` only when the PixLab deployment intentionally uses
a different network name. The overlay preserves WeKnora's own network and all
published host ports. It adds stable internal aliases:

| PixLab Nginx upstream | WeKnora service | Container port |
|---|---|---:|
| `weknora-frontend` | `frontend` | 80 |
| `weknora-app` | `app` | 8080 |

The overlay is optional. Do not include it for a standalone WeKnora deployment.

## Verification

From a container attached to `pixlab-internal`, verify both service names and
the exact workbench routes:

```bash
getent hosts weknora-frontend weknora-app
curl -fsS http://weknora-frontend/weknora-workbench/
curl -fsS http://weknora-app:8080/health
```

Then verify through the PixLab public origin:

```bash
curl -I https://PIXLab_HOST/weknora-workbench/
curl -i https://PIXLab_HOST/api/v1/pixlab-workbench/projects/UNKNOWN/context
curl -i https://PIXLab_HOST/api/health/ready
```

The workbench page should return WeKnora HTML. An unauthenticated workbench API
request should return the WeKnora workbench authentication error. The ordinary
PixLab health route must still return PixLab's response.

Also verify the effective PixLab Nginx configuration with `nginx -t` and
`nginx -T`. The two exact `^~` workbench locations must remain separate from
the generic PixLab `/api/` and SPA locations.

## Rollback

Redeploy `app` and `frontend` without the overlay to detach the shared network:

```bash
docker compose -f docker-compose.yml up -d --force-recreate app frontend
```

This does not remove WeKnora data volumes or the PixLab-owned external network.
