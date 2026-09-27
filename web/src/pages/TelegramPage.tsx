// Файл: web/src/pages/TelegramPage.tsx
// Вход по шагам и группы аккаунта; шаг, поля, ошибки и кнопки приходят с бэкенда.
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey, useScreen } from '../queries'
import type { TelegramScreen } from '../types'

function useTelegramAction<V>(path: (v: V) => string, body: (v: V) => unknown) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (v: V) => postJSON<TelegramScreen>(path(v), body(v)),
    onSuccess: (next) => queryClient.setQueryData(screenKey('telegram'), next),
  })
}

function LoginForm({ screen }: { screen: TelegramScreen }) {
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(screen.fields.map((f) => [f.key, f.value])),
  )
  const submit = useTelegramAction(() => '/api/telegram/submit', (v: Record<string, string>) => ({ values: v }))
  return (
    <form className="max-w-xl space-y-4" onSubmit={(e) => { e.preventDefault(); submit.mutate(values) }}>
      <p>{screen.hint}</p>
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
      {submit.error && <p className="text-red-600">{submit.error.message}</p>}
      <Button type="submit" disabled={submit.isPending}>
        {submit.isPending ? screen.saving_label : screen.submit_label}
      </Button>
    </form>
  )
}

function Groups({ screen }: { screen: TelegramScreen }) {
  const add = useTelegramAction(() => '/api/telegram/groups', (key: string) => ({ key }))
  const refresh = useTelegramAction(() => '/api/telegram/refresh', () => ({}))
  return (
    <section className="space-y-3">
      <p className="text-green-600">{screen.status}</p>
      <div className="flex items-center gap-2">
        <h2 className="font-semibold">{screen.groups_title}</h2>
        <Button variant="outline" disabled={refresh.isPending} onClick={() => refresh.mutate(undefined)}>
          {screen.refresh_label}
        </Button>
      </div>
      {(add.error ?? refresh.error) && <p className="text-red-600">{(add.error ?? refresh.error)?.message}</p>}
      {screen.groups_empty && <p className="text-neutral-500">{screen.groups_empty}</p>}
      {screen.groups.map((g) => (
        <div key={g.key} className="flex items-center gap-2 rounded-md border border-neutral-200 p-2 dark:border-neutral-800">
          <span>{g.title}</span>
          <span className="ml-auto text-sm text-neutral-500">{g.status}</span>
          {g.action && (
            <Button disabled={add.isPending} onClick={() => add.mutate(g.key)}>{g.action.label}</Button>
          )}
        </div>
      ))}
    </section>
  )
}

function BotSection({ screen }: { screen: TelegramScreen }) {
  const [token, setToken] = useState('')
  const save = useTelegramAction(() => '/api/telegram/bot', (t: string) => ({ token: t }))
  const f = screen.bot_field
  return (
    <form className="max-w-xl space-y-2 border-t border-neutral-200 pt-4 dark:border-neutral-800"
      onSubmit={(e) => { e.preventDefault(); save.mutate(token) }}>
      <h2 className="font-semibold">{screen.bot_title}</h2>
      <p className="text-sm text-neutral-500">{screen.bot_hint}</p>
      <p>{screen.bot_status}</p>
      <label className="block space-y-1">
        <span className="text-sm">{f.label}</span>
        <input type={f.kind} value={token} onChange={(e) => setToken(e.target.value)}
          className="w-full rounded-md border border-neutral-300 bg-transparent px-2 py-1 dark:border-neutral-700" />
        {f.error && <span className="text-sm text-red-600">{f.error}</span>}
      </label>
      {save.error && <p className="text-red-600">{save.error.message}</p>}
      <Button type="submit" disabled={save.isPending}>{save.isPending ? screen.saving_label : screen.bot_save_label}</Button>
    </form>
  )
}

// Offline — нет связи с Telegram: только повторить (кнопка «обновить» перечитывает экран).
function Offline({ screen }: { screen: TelegramScreen }) {
  const refresh = useTelegramAction(() => '/api/telegram/refresh', () => ({}))
  return (
    <Button variant="outline" disabled={refresh.isPending} onClick={() => refresh.mutate(undefined)}>
      {screen.refresh_label}
    </Button>
  )
}

function EditKeys({ screen }: { screen: TelegramScreen }) {
  const edit = useTelegramAction(() => '/api/telegram/keys', () => ({}))
  return (
    <Button variant="ghost" disabled={edit.isPending} onClick={() => edit.mutate(undefined)}>
      {screen.edit_keys_label}
    </Button>
  )
}

export function TelegramPage() {
  const query = useScreen<TelegramScreen>('telegram')
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-4">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          {s.message && <p className="text-amber-700 dark:text-amber-400">{s.message}</p>}
          {s.fields.length > 0 && <LoginForm key={s.hint} screen={s} />}
          {s.fields.length === 0 && s.status && <Groups screen={s} />}
          {s.fields.length === 0 && !s.status && <Offline screen={s} />}
          {s.edit_keys_label && <EditKeys screen={s} />}
          <BotSection screen={s} />
        </section>
      )}
    </ScreenView>
  )
}
