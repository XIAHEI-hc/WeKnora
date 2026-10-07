# MemoryLab MRA native workbench deployment

MemoryLab is an independent registered host of WeKnora's multi-platform native
project workbench. Read the
[multi-platform embedding architecture](./03-multi-platform-embedding.md)
before this host-specific deployment guide.

MemoryLab reuses the same constrained native workbench surface as PixLab while
keeping a separate host bridge, user namespace, project binding, and knowledge
scope. The browser sees the WeKnora UI through MemoryLab's own origin and never
chooses a tenant, knowledge base, or agent.

## Boundary

| Concern | Owner |
|---|---|
| Login, `knowledge:use` permission, one-time ticket | MemoryLab |
| Ticket exchange and continuous principal validation | MemoryLab and WeKnora backchannel |
| Project-to-knowledge-base mapping | WeKnora server-side binding table |
| Knowledge, chat, citations, and document UI | WeKnora native workbench |
| Browser-visible upstreams | Only `/weknora-workbench/` and `/api/v1/pixlab-workbench/` |

`/api/v1/pixlab-workbench/` remains the backward-compatible wire path shared by
the native workbench bundle. MemoryLab identity is selected server-side by the
fixed `MEMORYLAB_PROJECT_CODE`; it uses `X-MemoryLab-*` HMAC headers,
`memorylab:` user IDs, and the `memorylab-workbench` channel. It does not call
the PixLab authorization bridge.

## Project and knowledge scope

`MEMORYLAB_MRA` is the MemoryLab service project, not a shared cross-platform
knowledge pool. Its server-side binding fixes the MemoryLab tenant, the
`memorylab` knowledge base and the configured Agent. All document, chunk,
preview, citation, upload and chat requests are constrained to that binding.

MemoryLab uses `X-MemoryLab-*`, the `memorylab:` user namespace and the
`memorylab-workbench` channel. It must not reuse PixLab credentials,
`pixlab:` users or the CIS knowledge-base binding. A CIS-only question returning
no supporting evidence in MemoryLab is an expected and required isolation
result.

## Environment contract

Generate one random secret of at least 32 bytes and inject the same Key ID and
secret into both deployments. Never commit the secret.

MemoryLab:

```env
KNOWLEDGE_WORKBENCH_ENABLED=true
KNOWLEDGE_WORKBENCH_PROJECT_CODE=MEMORYLAB_MRA
KNOWLEDGE_WORKBENCH_PROJECT_NAME=MemoryLab MRA 知识服务
KNOWLEDGE_WORKBENCH_BINDING_REVISION=1
KNOWLEDGE_WORKBENCH_TICKET_REDIS_URL=redis://redis:6379/1
KNOWLEDGE_WORKBENCH_TICKET_TTL_SECONDS=60
KNOWLEDGE_WORKBENCH_REPLAY_TTL_SECONDS=120
WEKNORA_BACKCHANNEL_KEY_ID=<shared-key-id>
WEKNORA_BACKCHANNEL_SECRET=<shared-secret>
WEKNORA_UI_INTERNAL_URL=http://host.docker.internal:80
WEKNORA_APP_INTERNAL_URL=http://host.docker.internal:8080
```

WeKnora:

```env
MEMORYLAB_BRIDGE_ENABLED=true
MEMORYLAB_PROJECT_CODE=MEMORYLAB_MRA
MEMORYLAB_INTERNAL_BASE_URL=http://memorylab-backend:9000
MEMORYLAB_DOCKER_NETWORK=memorylab-internal
MEMORYLAB_BACKCHANNEL_KEY_ID=<shared-key-id>
MEMORYLAB_BACKCHANNEL_SECRET=<shared-secret>
MEMORYLAB_WORKBENCH_SESSION_TTL=3600
MEMORYLAB_BACKCHANNEL_TIMEOUT=10
MEMORYLAB_PUBLIC_ORIGIN=http://127.0.0.1:7891
```

`MEMORYLAB_PUBLIC_ORIGIN` must exactly match the browser origin. Use the real
HTTPS origin in production. The PixLab `PIXLAB_*` settings remain independent.

MemoryLab creates the external network named by `MEMORYLAB_DOCKER_NETWORK` and
publishes its backend on that network as `memorylab-backend`. Include
`docker-compose.memorylab.yml` when recreating the WeKnora App. This avoids
opening MemoryLab's loopback-only host port and avoids the ambiguous `backend`
service name used by other Compose projects.

## Server-controlled binding

Before changing data, record the existing CIS row. Substitute the verified
MemoryLab knowledge-base ID for `<MEMORYLAB_KB_ID>`:

```sql
SELECT project_code, project_name, tenant_id, knowledge_base_id, agent_id,
       status, revision
FROM pixlab_project_bindings
WHERE project_code IN ('WK-E2E-20261005', 'MEMORYLAB_MRA')
ORDER BY project_code;

INSERT INTO pixlab_project_bindings (
  project_code, project_name, tenant_id, knowledge_base_id, agent_id,
  status, revision, created_at, updated_at
)
VALUES (
  'MEMORYLAB_MRA', 'MemoryLab MRA 知识服务', 10000,
  '<MEMORYLAB_KB_ID>', 'builtin-quick-answer',
  'active', 1, now(), now()
)
ON CONFLICT (project_code) DO UPDATE SET
  project_name = EXCLUDED.project_name,
  tenant_id = EXCLUDED.tenant_id,
  knowledge_base_id = EXCLUDED.knowledge_base_id,
  agent_id = EXCLUDED.agent_id,
  status = EXCLUDED.status,
  revision = EXCLUDED.revision,
  updated_at = now();
```

Run the `SELECT` again and confirm the CIS row is unchanged. Never bind
`MEMORYLAB_MRA` to the CIS knowledge base.

## Deployment and verification

Recreate only the WeKnora API/UI and the MemoryLab stack; do not remove data
volumes:

```bash
cd /path/to/private_MEM_LAB
docker compose up -d --build

cd /path/to/WeKnora
docker compose -f docker-compose.yml -f DEPLOYMENT_IMAGE_OVERRIDE.yml \
  -f docker-compose.pixlab.yml -f docker-compose.memorylab.yml \
  up -d --no-deps app frontend
```

Acceptance requires all of the following:

1. A logged-in MemoryLab user sees `知识服务 → MRA知识库与问答` and enters the
   bound native page directly, without a project selector or a second login.
2. Context reports project `MEMORYLAB_MRA` and the intended MemoryLab/MRA
   knowledge base.
3. Browser traffic is limited to the two workbench path prefixes above; the
   ordinary WeKnora `/api/v1/*` surface is not exposed through MemoryLab.
4. MRA questions can cite only documents from the bound MemoryLab knowledge
   base. A CIS-specific question must not cite or reveal CIS documents.
5. New WeKnora chat sessions use a `memorylab:` user ID and project code
   `MEMORYLAB_MRA`; refresh resumes the same constrained session.
6. Revoking the MemoryLab login or `knowledge:use` permission invalidates the
   next WeKnora request. PixLab's existing native workbench still works.

## Rollback

Set `KNOWLEDGE_WORKBENCH_ENABLED=false` in MemoryLab and
`MEMORYLAB_BRIDGE_ENABLED=false` in WeKnora, then recreate the affected
services. This disables the entry and bridge without deleting knowledge bases,
documents, or conversations. Preserve the binding row unless the project is
being permanently decommissioned.
