// Файл: web/src/pages/TablePage.tsx
// Конструктор колонок таблицы Excel: фронт только держит правку формы; проверяет и сохраняет бэкенд.
import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey, useScreen } from '../queries'
import type { TableColumn, TableScreen } from '../types'

const inputClass = 'w-full rounded-md border border-neutral-300 bg-transparent px-2 py-1 dark:border-neutral-700'

function Editor({ s }: { s: TableScreen }) {
  const queryClient = useQueryClient()
  const [cols, setCols] = useState<TableColumn[]>(s.columns)
  const [include, setInclude] = useState(s.include_queries)
  useEffect(() => {
    setCols(s.columns)
    setInclude(s.include_queries)
  }, [s])
  const save = useMutation({
    mutationFn: () => postJSON<TableScreen>('/api/table', { columns: cols, include_queries: include }),
    onSuccess: (next) => queryClient.setQueryData(screenKey('table'), next),
  })
  const edit = (i: number, patch: Partial<TableColumn>) => setCols(cols.map((c, j) => (j === i ? { ...c, ...patch } : c)))
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-[1fr_10rem_2fr_auto] gap-2 text-sm text-neutral-500">
        <span>{s.name_label}</span><span>{s.type_label}</span><span>{s.hint_label}</span><span />
      </div>
      {cols.map((c, i) => (
        <div key={i} className="grid grid-cols-[1fr_10rem_2fr_auto] gap-2">
          <input className={inputClass} value={c.name} onChange={(e) => edit(i, { name: e.target.value })} />
          <select className={inputClass} value={c.type} onChange={(e) => edit(i, { type: e.target.value })}>
            {s.types.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
          </select>
          <input className={inputClass} value={c.hint} onChange={(e) => edit(i, { hint: e.target.value })} />
          <Button variant="ghost" onClick={() => setCols(cols.filter((_, j) => j !== i))}>{s.remove_label}</Button>
        </div>
      ))}
      <Button variant="outline" onClick={() => setCols([...cols, s.new_column])}>{s.add_label}</Button>
      <label className="flex items-center gap-2">
        <input type="checkbox" checked={include} onChange={(e) => setInclude(e.target.checked)} />
        {s.include_label}
      </label>
      <div className="flex items-center gap-3">
        <Button disabled={save.isPending} onClick={() => save.mutate()}>{save.isPending ? s.saving_label : s.save_label}</Button>
        {s.message && <span className="text-neutral-600 dark:text-neutral-300">{s.message}</span>}
        {s.error && <span className="text-red-600">{s.error}</span>}
        {save.error && <span className="text-red-600">{save.error.message}</span>}
      </div>
    </div>
  )
}

export function TablePage() {
  const query = useScreen<TableScreen>('table')
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-3">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          <p className="text-neutral-500">{s.hint}</p>
          <Editor s={s} />
        </section>
      )}
    </ScreenView>
  )
}
