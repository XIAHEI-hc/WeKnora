export type WorkbenchCapability = 'read' | 'upload' | 'chat'

export interface WorkbenchContext {
  project_code: string
  project_name: string
  knowledge_base: { id: string; name: string }
  agent_id: string
  capabilities: WorkbenchCapability[]
  readiness: { parser: string; retrieval: string; chat: string }
}

export interface WorkbenchDocument {
  id: string
  title: string
  description: string
  file_name: string
  folder_path: string
  file_type: string
  file_size: number
  parse_status: string
  summary_status: string
  pending_subtasks_count: number
  error_message?: string
  created_at: string
  updated_at: string
  processed_at?: string
  last_activity_at?: string
  stall_state?: string
}

export interface DocumentPage {
  total: number
  page: number
  page_size: number
  documents: WorkbenchDocument[]
}

export interface WorkbenchFolderNode {
  path: string
  name: string
  document_count: number
  total_count: number
  children?: WorkbenchFolderNode[]
}

export interface WorkbenchFolderTree {
  root_document_count: number
  total_document_count: number
  folders: WorkbenchFolderNode[]
}

export interface WorkbenchSession {
  id: string
  title: string
  description: string
  created_at: string
  updated_at: string
}

export interface WorkbenchCitation {
  id: string
  document_id: string
  document_title: string
  document_file_name: string
  content: string
  chunk_index: number
  start_at: number
  end_at: number
  source_locators?: unknown[]
}

export interface WorkbenchChunk {
  id: string
  knowledge_id: string
  knowledge_base_id: string
  content: string
  chunk_index: number
  chunk_type: string
  parent_chunk_id?: string
  content_revision?: number
  source_locators?: unknown[]
  image_info?: string
}

export interface WorkbenchMessage {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  completed: boolean
  fallback?: boolean
  citations: WorkbenchCitation[]
  created_at: string
  updated_at: string
}

export interface WorkbenchStreamEvent {
  id?: string
  response_type: string
  content?: string
  done?: boolean
  assistant_message_id?: string
  knowledge_references?: Array<{
    id?: string
    knowledge_id?: string
    knowledge_title?: string
    knowledge_filename?: string
    content?: string
    chunk_index?: number
  }>
  data?: Record<string, unknown>
}

interface Envelope<T> {
  data: T
  request_id?: string
}

export class WorkbenchApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId = '',
  ) {
    super(message)
  }
}

let csrfToken = ''

function projectPath(projectCode: string, suffix: string) {
  return `/api/v1/pixlab-workbench/projects/${encodeURIComponent(projectCode)}${suffix}`
}

async function request<T>(url: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (csrfToken && !['GET', 'HEAD'].includes((init.method || 'GET').toUpperCase())) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(url, { ...init, headers, credentials: 'same-origin' })
  const payload = await response.json().catch(() => ({})) as Partial<Envelope<T>> & {
    code?: string
    message?: string
    request_id?: string
  }
  if (!response.ok) {
    throw new WorkbenchApiError(
      response.status,
      payload.code || 'WORKBENCH_REQUEST_FAILED',
      payload.message || '项目知识服务请求失败',
      payload.request_id,
    )
  }
  return payload.data as T
}

async function responseError(response: Response) {
  const payload = await response.json().catch(() => ({})) as {
    code?: string
    message?: string
    request_id?: string
  }
  return new WorkbenchApiError(
    response.status,
    payload.code || 'WORKBENCH_REQUEST_FAILED',
    payload.message || '项目知识服务请求失败',
    payload.request_id,
  )
}

export async function createWorkbenchSession(ticket: string, projectCode: string) {
  const result = await request<{ csrf_token: string; expires_in: number }>(
    '/api/v1/pixlab-workbench/session',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ticket, project_code: projectCode }),
    },
  )
  csrfToken = result.csrf_token
  return result
}

export function getContext(projectCode: string) {
  return request<WorkbenchContext>(projectPath(projectCode, '/context'))
}

export function listFolders(projectCode: string) {
  return request<WorkbenchFolderTree>(projectPath(projectCode, '/folders'))
}

export function listDocuments(
  projectCode: string,
  options: {
    page?: number
    size?: number
    folder?: string
    query?: string
    sortBy?: string
    sortOrder?: string
    parseStatus?: string
  } = {},
) {
  const query = new URLSearchParams({
    page: String(options.page || 1),
    size: String(options.size || 100),
  })
  if (options.folder) query.set('folder', options.folder)
  if (options.query) query.set('query', options.query)
  if (options.sortBy) query.set('sort_by', options.sortBy)
  if (options.sortOrder) query.set('sort_order', options.sortOrder)
  if (options.parseStatus) query.set('status', options.parseStatus)
  return request<DocumentPage>(projectPath(projectCode, `/documents?${query}`))
}

export function getDocument(projectCode: string, documentId: string) {
  return request<WorkbenchDocument>(
    projectPath(projectCode, `/documents/${encodeURIComponent(documentId)}`),
  )
}

export function uploadDocument(projectCode: string, file: File, relativeName: string) {
  const body = new FormData()
  body.set('file', file)
  body.set('fileName', relativeName)
  return request<{ id: string; file_name: string; folder_path: string; parse_status: string }>(
    projectPath(projectCode, '/documents'),
    { method: 'POST', body },
  )
}

export function reparseDocument(projectCode: string, documentId: string) {
  return request<{ id: string; parse_status: string }>(
    projectPath(projectCode, `/documents/${encodeURIComponent(documentId)}/reparse`),
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' },
  )
}

export function documentPreviewUrl(projectCode: string, documentId: string) {
  return projectPath(projectCode, `/documents/${encodeURIComponent(documentId)}/preview`)
}

export function getDocumentStages(projectCode: string, documentId: string) {
  return request<Record<string, unknown>>(
    projectPath(projectCode, `/documents/${encodeURIComponent(documentId)}/stages`),
  )
}

export function getDocumentStatuses(projectCode: string, documentIds: string[]) {
  return request<{ documents: WorkbenchDocument[] }>(projectPath(projectCode, '/documents/status'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids: documentIds }),
  })
}

export async function getDocumentPreview(projectCode: string, documentId: string) {
  const response = await fetch(documentPreviewUrl(projectCode, documentId), {
    credentials: 'same-origin',
  })
  if (!response.ok) throw await responseError(response)
  return response.blob()
}

export function getDocumentChunk(
  projectCode: string,
  documentId: string,
  chunkId: string,
) {
  return request<WorkbenchChunk>(projectPath(
    projectCode,
    `/documents/${encodeURIComponent(documentId)}/chunks/${encodeURIComponent(chunkId)}`,
  ))
}

export function getProjectChunk(projectCode: string, chunkId: string) {
  return request<WorkbenchChunk>(
    projectPath(projectCode, `/chunks/${encodeURIComponent(chunkId)}`),
  )
}

export function listDocumentChunks(projectCode: string, documentId: string, page = 1, size = 25) {
  return request<{ chunks: WorkbenchChunk[]; total: number; page: number; page_size: number }>(
    projectPath(
      projectCode,
      `/documents/${encodeURIComponent(documentId)}/chunks?page=${page}&size=${size}`,
    ),
  )
}

export function listChatSessions(projectCode: string, page = 1, size = 100) {
  return request<{ sessions: WorkbenchSession[]; total: number; page: number; page_size: number }>(
    projectPath(projectCode, `/sessions?page=${page}&size=${size}`),
  )
}

export function createChatSession(projectCode: string, title = '') {
  return request<WorkbenchSession>(projectPath(projectCode, '/sessions'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

export function deleteChatSession(projectCode: string, sessionId: string) {
  return request<{ deleted: boolean }>(
    projectPath(projectCode, `/sessions/${encodeURIComponent(sessionId)}`),
    { method: 'DELETE' },
  )
}

export function getChatSession(projectCode: string, sessionId: string) {
  return request<WorkbenchSession>(
    projectPath(projectCode, `/sessions/${encodeURIComponent(sessionId)}`),
  )
}

export function listChatMessages(projectCode: string, sessionId: string, limit = 100, before = '') {
  const query = new URLSearchParams({ limit: String(limit) })
  if (before) query.set('before_time', before)
  return request<{ messages: WorkbenchMessage[] }>(
    projectPath(projectCode, `/sessions/${encodeURIComponent(sessionId)}/messages?${query}`),
  )
}

export function stopChatAnswer(projectCode: string, sessionId: string, messageId: string) {
  return request<unknown>(
    projectPath(projectCode, `/sessions/${encodeURIComponent(sessionId)}/stop`),
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message_id: messageId }),
    },
  )
}

export async function streamChatAnswer(
  projectCode: string,
  sessionId: string,
  query: string,
  onEvent: (event: WorkbenchStreamEvent) => void,
  signal?: AbortSignal,
) {
  const response = await fetch(
    projectPath(projectCode, `/sessions/${encodeURIComponent(sessionId)}/answers`),
    {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        ...(csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
      },
      body: JSON.stringify({ query }),
      signal,
    },
  )
  if (!response.ok) throw await responseError(response)
  if (!response.body) throw new WorkbenchApiError(502, 'STREAM_UNAVAILABLE', '问答流不可用')

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let terminalEventReceived = false
  const consume = (frame: string) => {
    const data = frame.split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trimStart())
      .join('\n')
    if (!data || data === '[DONE]') return
    let event: WorkbenchStreamEvent
    try {
      event = JSON.parse(data) as WorkbenchStreamEvent
    } catch {
      // Ignore non-JSON heartbeat frames; the server's message events are JSON.
      return
    }
    if (!event || typeof event.response_type !== 'string') return
    if (
      event.response_type === 'complete' ||
      event.response_type === 'error' ||
      (event.response_type === 'answer' && event.done === true)
    ) {
      terminalEventReceived = true
    }
    onEvent(event)
  }

  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done }).replace(/\r\n/g, '\n')
    let boundary = buffer.indexOf('\n\n')
    while (boundary >= 0) {
      consume(buffer.slice(0, boundary))
      buffer = buffer.slice(boundary + 2)
      boundary = buffer.indexOf('\n\n')
    }
    if (done) break
  }
  if (buffer.trim()) consume(buffer)
  if (!terminalEventReceived && !signal?.aborted) {
    throw new WorkbenchApiError(502, 'STREAM_INTERRUPTED', '问答连接在回答完成前中断')
  }
}

export function clearWorkbenchSession() {
  csrfToken = ''
}
