export function getApiBaseUrl(): string {
  // LocalHub plugin patch (2026-04-29): respect vite's BASE_URL so that
  // axios calls work at `/app/weknora/` (LocalHub reverse proxy). Without
  // this · axios hits `/api/v1/...` at LocalHub root · gets 404 "Cannot
  // POST". Strip trailing slash so axios doesn't produce `/app/weknora//api/v1/...`.
  // See: plugins/weknora/patches/api-base-baseurl.patch
  const configuredBase = import.meta.env.BASE_URL || '/';
  // The embedded workbench is served under /weknora-workbench/, but its API
  // proxy still lives at the host root (/api/v1). Axios otherwise prefixes
  // every Wiki/KB request with /weknora-workbench and those requests never
  // reach the workbench API proxy.
  const base = configuredBase.includes('/weknora-workbench')
    ? '/'
    : configuredBase.replace(/\/+$/, '');
  return base;
}
