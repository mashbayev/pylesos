// Файл: web/src/pages/HomePage.tsx
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey, useScreen } from '../queries'
import type { HomeScreen } from '../types'

export function HomePage() {
  const query = useScreen<HomeScreen>('home')
  const queryClient = useQueryClient()
  const act = useMutation({
    mutationFn: (v: { id: number; action: string }) => postJSON<HomeScreen>(`/api/sources/${v.id}/${v.action}`, {}),
    onSuccess: (next) => queryClient.setQueryData(screenKey('home'), next),
  })
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-3">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          {s.banners.map((b) => (
            <div key={b.job_id} className="rounded-md border border-amber-400 bg-amber-50 p-3 dark:bg-amber-950">
              {b.text}
            </div>
          ))}
          {s.empty_text && <p className="text-neutral-500">{s.empty_text}</p>}
          {act.error && <p className="text-red-600">{act.error.message}</p>}
          {s.telegram.length > 0 && <h2 className="font-semibold">{s.telegram_title}</h2>}
          {s.telegram.map((row) => (
            <div key={row.id} className="flex items-center gap-3 rounded-md border border-neutral-200 p-3 dark:border-neutral-800">
              <div className="space-y-0.5">
                <div className="font-medium">{row.title}</div>
                {row.lines.map((line) => <div key={line} className="text-sm text-neutral-500">{line}</div>)}
              </div>
              <div className="ml-auto flex gap-2">
                {row.actions.map((a) => (
                  <Button key={a.id} variant={a.tone === 'primary' ? 'default' : 'outline'} disabled={act.isPending}
                    onClick={() => act.mutate({ id: row.id, action: a.id })}>{a.label}</Button>
                ))}
              </div>
            </div>
          ))}
        </section>
      )}
    </ScreenView>
  )
}
