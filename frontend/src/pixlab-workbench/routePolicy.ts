export function projectRouteBase(projectCode: string) {
  return `/projects/${encodeURIComponent(projectCode)}`
}

export function isAllowedProjectRoute(pathname: string, projectCode: string) {
  const base = projectRouteBase(projectCode)
  if (pathname === base || pathname === `${base}/` || pathname === `${base}/knowledge`) return true
  if (pathname === `${base}/chat/new`) return true
  return pathname.startsWith(`${base}/chat/`) && pathname.slice(`${base}/chat/`.length).length > 0 &&
    !pathname.slice(`${base}/chat/`.length).includes('/')
}
