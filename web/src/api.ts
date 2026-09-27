// Файл: web/src/api.ts
// Транспорт к бэкенду: только запросы и ошибки, никакой логики (П3).
const token = new URLSearchParams(window.location.search).get('t') ?? ''

export class ApiError extends Error {}

// Бэкенд недоступен — единственный текст, который фронт не может получить от бэкенда (09-frontend.md).
const NO_CONNECTION = 'Нет связи с программой'

async function send(path: string, init: RequestInit): Promise<Response> {
  try {
    return await fetch(path, init)
  } catch {
    throw new ApiError(NO_CONNECTION)
  }
}

async function parse<T>(res: Response): Promise<T> {
  const body: unknown = await res.json().catch(() => null)
  if (!res.ok) {
    const text =
      typeof body === 'object' && body !== null && 'error' in body
        ? String((body as { error: unknown }).error)
        : res.statusText
    throw new ApiError(text)
  }
  return body as T
}

export async function getJSON<T>(path: string): Promise<T> {
  return parse<T>(await send(path, { headers: { 'X-Pylesos-Token': token } }))
}

export async function postJSON<T>(path: string, body: unknown): Promise<T> {
  return parse<T>(
    await send(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Pylesos-Token': token },
      body: JSON.stringify(body),
    }),
  )
}

// withToken — адрес для <img> и EventSource: заголовок не передать, токен идёт параметром ?t=.
export function withToken(path: string): string {
  return `${path}?t=${encodeURIComponent(token)}`
}

export function eventsURL(): string {
  return `/api/events?t=${encodeURIComponent(token)}`
}
