# PixLab native workbench deployment

PixLab is one registered host of WeKnora's multi-platform native project
workbench, not the owner of the embedding capability. Read the
[multi-platform embedding architecture](./03-multi-platform-embedding.md)
before this host-specific deployment guide.

The PixLab workbench is served from the same public origin as PixLab. PixLab's
gateway forwards only these two path prefixes to WeKnora:

- `/weknora-workbench/` to the WeKnora `frontend` service.
- `/api/v1/pixlab-workbench/` to the WeKnora `app` service.

The ordinary WeKnora `/api/v1/*` surface must not be exposed through the
PixLab gateway.

## Project and knowledge scope

Each PixLab business project resolves to a server-controlled WeKnora binding.
The binding fixes `tenant_id`, `knowledge_base_id`, `agent_id`, status and
revision. The browser may select only a PixLab project returned by PixLab's
authorization service; it cannot supply or override the knowledge base or
agent.

The verified CIS project `WK-E2E-20261005` is bound to the CIS knowledge base.
That binding is independent from MemoryLab's `MEMORYLAB_MRA` binding. Questions
asked from PixLab must not retrieve or cite MemoryLab-only documents.

PixLab uses the `X-PixLab-*` backchannel headers, the `pixlab:` WeKnora user
namespace and the `pixlab-workbench` channel. These values are host-specific
and must not be reused by another platform.

## Verified release

The native project workbench was built and exercised on 2026-10-07 from the
following revisions:

| Repository | Branch | Revision |
|---|---|---|
| `XIAHEI-hc/paravite_pixlab` | `main` | `8083e07a9f74917bee2966ab7527f2413cbe73b3` |
| `XIAHEI-hc/WeKnora` | `feature/embedding` | `eec12a0bc7d6c0f28a6fd7c6ac6797efa6f47cad` |

The locally verified WeKnora images are immutable by digest:

| Service | Tag | Image digest |
|---|---|---|
| API | `weknora-app:pixlab-native-20261007-eec12a0` | `sha256:12698d6a65dc43ed0e97f41292d05601de90f95b07f1aa314e81324b7d194508` |
| UI | `weknora-ui:pixlab-native-20261007-eec12a0` | `sha256:c0b907644fe6984467b5f02f4ce86fc5ec42d5249526befdc02402a4ad41fcc8` |

These are local deployment tags, not public registry coordinates. Publish the
same image IDs to the deployment registry before using the tags on another
host.

## Environment contract

PixLab and WeKnora must use the same key ID and secret. Generate the secret as
at least 32 cryptographically random bytes and inject it through the deployment
secret store; never commit its value.

| Owner | Variable | Purpose |
|---|---|---|
| PixLab | `AI_WORKBENCH_ENABLED=true` | Enables the project selector, ticket issuer, and internal authorization endpoint. |
| PixLab | `AI_WORKBENCH_TICKET_REDIS_URL` | Dedicated Redis database for single-use bootstrap tickets. |
| PixLab | `AI_WORKBENCH_TICKET_TTL_SECONDS` | Short ticket lifetime; the reference value is `60`. |
| PixLab | `AI_WORKBENCH_INTERNAL_REPLAY_TTL_SECONDS` | Replay cache lifetime; the reference value is `120`. |
| PixLab | `WEKNORA_INTERNAL_BASE_URL` | Backchannel URL of the WeKnora API. |
| PixLab | `WEKNORA_BACKCHANNEL_KEY_ID` | Identifier for the shared backchannel credential. |
| PixLab | `WEKNORA_BACKCHANNEL_SECRET` | Shared HMAC secret. |
| WeKnora | `PIXLAB_BRIDGE_ENABLED=true` | Enables only the PixLab workbench API surface. |
| WeKnora | `PIXLAB_INTERNAL_BASE_URL` | Backchannel URL of the PixLab API. |
| WeKnora | `PIXLAB_BACKCHANNEL_KEY_ID` | Must equal PixLab's key ID. |
| WeKnora | `PIXLAB_BACKCHANNEL_SECRET` | Must equal PixLab's HMAC secret. |
| WeKnora | `PIXLAB_WORKBENCH_SESSION_TTL` | Redis-backed workbench session lifetime; the reference value is `3600`. |
| WeKnora | `PIXLAB_BACKCHANNEL_TIMEOUT` | Authorization backchannel timeout; the reference value is `10`. |
| WeKnora | `PIXLAB_PUBLIC_ORIGIN` | Exact browser origin of PixLab, with no trailing slash. |

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
curl -i https://PIXLab_HOST/api/health
```

The workbench page should return WeKnora HTML. An unauthenticated workbench API
request should return the WeKnora workbench authentication error. The ordinary
PixLab health route must still return PixLab's response.

Also verify the effective PixLab Nginx configuration with `nginx -t` and
`nginx -T`. The two exact `^~` workbench locations must remain separate from
the generic PixLab `/api/` and SPA locations.

The 2026-10-07 local acceptance run also verified:

- PixLab SPA and WeKnora workbench deep-link refreshes return HTML with status
  200 through the production Nginx image.
- PixLab `/api/health` returns 200 through the generic API proxy.
- An unauthenticated workbench context and document-preview request reaches
  WeKnora and returns 401.
- A 2 MiB upload request reaches WeKnora and returns an application-level 403,
  rather than Nginx 413; the gateway limit is explicitly 256 MiB.
- A real project question returned the expected knowledge-base values and
  opened both the native citation drawer and source preview.
- Workbench data requests used only `/api/v1/pixlab-workbench/*`; no ordinary
  WeKnora `/api/v1/*` endpoint was exposed through PixLab.
- Reloading the iframe used one `/session/resume` request, made no second
  `/session` exchange, and caused PixLab to mint a fresh unused bootstrap
  ticket for the new iframe nonce.

## Rollback

To roll back the integrated deployment, restore the previously recorded image
tags or digests in the deployment override and recreate only `app` and
`frontend`:

```bash
docker compose \
  -f docker-compose.yml \
  -f DEPLOYMENT_IMAGE_OVERRIDE.yml \
  -f docker-compose.pixlab.yml \
  up -d --no-deps app frontend
```

To disable the integration completely, set `AI_WORKBENCH_ENABLED=false` in
PixLab and `PIXLAB_BRIDGE_ENABLED=false` in WeKnora, then recreate the affected
services. Existing WeKnora knowledge bases and conversations remain intact.

Redeploy `app` and `frontend` without the overlay to detach the shared network:

```bash
docker compose -f docker-compose.yml up -d --force-recreate app frontend
```

This does not remove WeKnora data volumes or the PixLab-owned external network.
