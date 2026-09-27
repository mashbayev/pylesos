// Файл: web/src/queries.ts
// Один queryKey — одна форма данных: ключ ['screen', имя] всегда читается через useScreen.
import { useQuery } from '@tanstack/react-query'
import { getJSON } from './api'

export const screenKey = (name: string) => ['screen', name] as const

export function useScreen<T>(name: string) {
  return useQuery({ queryKey: screenKey(name), queryFn: () => getJSON<T>(`/api/screens/${name}`) })
}
