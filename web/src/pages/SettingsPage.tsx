// Файл: web/src/pages/SettingsPage.tsx
// Поля и их ошибки приходят с бэкенда; фронт только держит набранный текст до отправки.
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey, useScreen } from '../queries'
import type { SettingsScreen } from '../types'

function SettingsForm({ screen }: { screen: SettingsScreen }) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(screen.fields.map((f) => [f.key, f.value])),
  )
  const save = useMutation({
    mutationFn: () => postJSON<SettingsScreen>('/api/settings', { values }),
    onSuccess: (next) => queryClient.setQueryData(screenKey('settings'), next),
  })
  return (
    <form className="max-w-xl space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
      <h1 className="text-xl font-semibold">{screen.title}</h1>
      <div className="text-sm">
        <span className="text-neutral-500">{screen.ollama_label}: </span>
        <span>{screen.ollama_status}</span>
      </div>
      {screen.fields.map((f) => (
        <label key={f.key} className="block space-y-1">
          <span className="text-sm">{f.label}</span>
          <input
            type={f.kind}
            value={values[f.key] ?? ''}
            onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
            className="w-full rounded-md border border-neutral-300 bg-transparent px-2 py-1 dark:border-neutral-700"
          />
          {f.error && <span className="text-sm text-red-600">{f.error}</span>}
        </label>
      ))}
      {save.error && <p className="text-red-600">{save.error.message}</p>}
      {screen.message && <p className="text-green-600">{screen.message}</p>}
      <Button type="submit" disabled={save.isPending}>
        {save.isPending ? screen.saving_label : screen.save_label}
      </Button>
    </form>
  )
}

export function SettingsPage() {
  const query = useScreen<SettingsScreen>('settings')
  return <ScreenView query={query}>{(s) => <SettingsForm screen={s} />}</ScreenView>
}
