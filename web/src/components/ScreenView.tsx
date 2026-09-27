// Файл: web/src/components/ScreenView.tsx
// Общая обёртка экрана: загрузка → спиннер, ошибка → текст от бэкенда, данные → children.
import type { ReactNode } from 'react'
import type { UseQueryResult } from '@tanstack/react-query'
import { Spinner } from './Spinner'

export function ScreenView<T>({ query, children }: { query: UseQueryResult<T, Error>; children: (data: T) => ReactNode }) {
  if (query.isLoading) return <Spinner />
  if (query.error) return <p className="text-red-600">{query.error.message}</p>
  if (query.data === undefined) return null
  return <>{children(query.data)}</>
}
