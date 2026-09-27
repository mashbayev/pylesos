// Файл: web/src/pages/FormsPage.tsx
// Формы документов: список, загрузка образца, разметка рамки и полей мышью.
// Фронт только переводит движение мыши в доли размера образца и отправляет их; проверяет всё бэкенд.
import { useRef, useState, type PointerEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getJSON, postJSON, withToken } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey } from '../queries'
import type { FormEditor, FormsScreen } from '../types'

const inputClass = 'w-full rounded-md border border-neutral-300 bg-transparent px-2 py-1 dark:border-neutral-700'

function useFormsAction<V>(path: (v: V) => string, body: (v: V) => unknown) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (v: V) => postJSON<FormsScreen>(path(v), body(v)),
    onSuccess: (next) => queryClient.setQueryData(screenKey('forms'), next),
  })
}

// readBase64 — файл образца в base64 (транспорт: бэкенд принимает только JSON).
function readBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result).split(',')[1] ?? '')
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

function CreateForm({ screen }: { screen: FormsScreen }) {
  const [name, setName] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const create = useFormsAction(() => '/api/forms', (v: { name: string; file_name: string; data_base64: string }) => v)
  const submit = async () => {
    const data = file ? await readBase64(file) : ''
    create.mutate({ name, file_name: file?.name ?? '', data_base64: data })
  }
  return (
    <form className="max-w-xl space-y-2" onSubmit={(e) => { e.preventDefault(); void submit() }}>
      <label className="block space-y-1">
        <span className="text-sm">{screen.name_label}</span>
        <input value={name} onChange={(e) => setName(e.target.value)} className={inputClass} />
      </label>
      <label className="block space-y-1">
        <span className="text-sm">{screen.file_label}</span>
        <input type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
      </label>
      {create.error && <p className="text-red-600">{create.error.message}</p>}
      <Button type="submit" disabled={create.isPending}>{create.isPending ? screen.saving_label : screen.create_label}</Button>
    </form>
  )
}

interface Rect { x: number; y: number; w: number; h: number }

function Editor({ editor }: { editor: FormEditor }) {
  const [target, setTarget] = useState(editor.targets[0]?.key ?? '')
  const [drag, setDrag] = useState<{ x0: number; y0: number; rect: Rect } | null>(null)
  const [name, setName] = useState(editor.name)
  const area = useRef<HTMLDivElement>(null)
  const zone = useFormsAction(() => `/api/forms/${editor.id}/zone`, (v: Rect & { key: string }) => v)
  const clear = useFormsAction(() => `/api/forms/${editor.id}/clear`, (key: string) => ({ key }))
  const rename = useFormsAction(() => `/api/forms/${editor.id}/rename`, (n: string) => ({ name: n }))
  const remove = useFormsAction(() => `/api/forms/${editor.id}/delete`, () => ({}))
  // point — положение указателя в долях образца; за краем — к краю (рамка часто совпадает со всей картинкой).
  const point = (e: PointerEvent) => {
    const b = area.current?.getBoundingClientRect()
    if (!b) return { x: 0, y: 0 }
    const clamp = (v: number) => Math.min(1, Math.max(0, v))
    return { x: clamp((e.clientX - b.left) / b.width), y: clamp((e.clientY - b.top) / b.height) }
  }
  const down = (e: PointerEvent<HTMLDivElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId) // отпускание за картинкой тоже завершает отметку
    const p = point(e)
    setDrag({ x0: p.x, y0: p.y, rect: { x: p.x, y: p.y, w: 0, h: 0 } })
  }
  const move = (e: PointerEvent) => {
    if (!drag) return
    const p = point(e)
    setDrag({ ...drag, rect: { x: Math.min(drag.x0, p.x), y: Math.min(drag.y0, p.y), w: Math.abs(p.x - drag.x0), h: Math.abs(p.y - drag.y0) } })
  }
  const up = () => {
    if (drag && drag.rect.w > 0 && drag.rect.h > 0) zone.mutate({ key: target, ...drag.rect }) // простой клик — не отметка
    setDrag(null)
  }
  const error = zone.error ?? clear.error ?? rename.error ?? remove.error
  return (
    <section className="space-y-3">
      <p className="text-sm text-neutral-500">{editor.hint}</p>
      <div className="flex flex-wrap gap-2">
        {editor.targets.map((t) => (
          <Button key={t.key} variant={t.key === target ? 'default' : 'outline'} onClick={() => setTarget(t.key)}>
            <span className="mr-1 inline-block h-3 w-3 rounded-sm" style={{ background: t.color }} />
            {t.label}{t.marked ? ' ✓' : ''}
          </Button>
        ))}
        <Button variant="ghost" disabled={clear.isPending} onClick={() => clear.mutate(target)}>{editor.clear_label}</Button>
      </div>
      {error && <p className="text-red-600">{error.message}</p>}
      <div ref={area} className="relative inline-block cursor-crosshair touch-none select-none"
        onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={() => setDrag(null)}>
        <img src={withToken(editor.sample_url)} alt={editor.name} draggable={false} className="max-w-full" />
        {editor.boxes.map((b) => (
          <div key={b.key} className="absolute border-2" title={b.label}
            style={{ left: `${b.x * 100}%`, top: `${b.y * 100}%`, width: `${b.w * 100}%`, height: `${b.h * 100}%`, borderColor: b.color }}>
            <span className="absolute -top-5 left-0 whitespace-nowrap px-1 text-xs text-white" style={{ background: b.color }}>{b.label}</span>
          </div>
        ))}
        {drag && (
          <div className="absolute border-2 border-dashed border-neutral-900 dark:border-white"
            style={{ left: `${drag.rect.x * 100}%`, top: `${drag.rect.y * 100}%`, width: `${drag.rect.w * 100}%`, height: `${drag.rect.h * 100}%` }} />
        )}
      </div>
      <form className="flex max-w-xl gap-2" onSubmit={(e) => { e.preventDefault(); rename.mutate(name) }}>
        <input value={name} onChange={(e) => setName(e.target.value)} className={inputClass} />
        <Button type="submit" variant="outline" disabled={rename.isPending}>{editor.rename_label}</Button>
        <Button type="button" variant="danger" disabled={remove.isPending} onClick={() => remove.mutate(undefined)}>{editor.delete_label}</Button>
      </form>
    </section>
  )
}

export function FormsPage() {
  const queryClient = useQueryClient()
  // Ключ ['screen','forms'] — одна форма данных. Какая форма открыта, знает последний ответ бэкенда
  // (после «Добавить» он сам открывает новую) — перечитываем с ней же.
  const query = useQuery({
    queryKey: screenKey('forms'),
    queryFn: () => {
      const open = queryClient.getQueryData<FormsScreen>(screenKey('forms'))?.editor?.id ?? 0
      return getJSON<FormsScreen>(`/api/screens/forms?id=${open}`)
    },
  })
  const open = (id: number) => {
    void getJSON<FormsScreen>(`/api/screens/forms?id=${id}`).then((next) => queryClient.setQueryData(screenKey('forms'), next))
  }
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-4">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          <p className="text-sm text-neutral-500">{s.hint}</p>
          {s.message && <p className="text-amber-700 dark:text-amber-400">{s.message}</p>}
          {s.empty_text && <p className="text-neutral-500">{s.empty_text}</p>}
          {s.forms.map((f) => (
            <div key={f.id} className={`flex items-center gap-3 rounded-md border p-2 ${f.selected ? 'border-neutral-900 dark:border-neutral-100' : 'border-neutral-200 dark:border-neutral-800'}`}>
              <span className="font-medium">{f.name}</span>
              <span className="text-sm text-neutral-500">{f.status}</span>
              <Button className="ml-auto" variant="outline" onClick={() => open(f.id)}>{f.open_label}</Button>
            </div>
          ))}
          {s.editor && <Editor key={s.editor.id} editor={s.editor} />}
          <CreateForm screen={s} />
        </section>
      )}
    </ScreenView>
  )
}
