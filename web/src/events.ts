// Файл: web/src/events.ts
// Сигнал от бэкенда «данные изменились» → перечитать открытые экраны.
import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { eventsURL } from './api'

export function useServerEvents(enabled: boolean): void {
  const queryClient = useQueryClient()
  useEffect(() => {
    if (!enabled) return
    const source = new EventSource(eventsURL())
    // Какие экраны перечитать, решает бэкенд (поле screens события).
    source.onmessage = (e: MessageEvent<string>) => {
      const event = JSON.parse(e.data) as { screens: string[] | null }
      for (const screen of event.screens ?? []) {
        void queryClient.invalidateQueries({ queryKey: ['screen', screen] })
      }
    }
    return () => source.close()
  }, [queryClient, enabled])
}
