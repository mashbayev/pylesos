// Файл: web/src/pages/PeoplePage.tsx
// Люди: поиск, карточка, «возможно, один человек», ручная привязка. Всё решает бэкенд;
// фронт держит только, какая вкладка, поиск и человек открыты.
import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getJSON, postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey } from '../queries'
import type { CandidateRow, MentionRow, PeopleScreen, PersonCard, UnlinkedRow } from '../types'

const inputClass = 'rounded-md border border-neutral-300 bg-transparent px-2 py-1 dark:border-neutral-700'
const boxClass = 'rounded-md border border-neutral-200 p-3 dark:border-neutral-800'

interface View { tab: string; q: string; id: number }

const screenURL = (v: View) =>
  `/api/screens/people?tab=${encodeURIComponent(v.tab)}&q=${encodeURIComponent(v.q)}&id=${v.id}`

function usePeopleAction<V>(action: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (v: V) => postJSON<PeopleScreen>(`/api/people/${action}`, v),
    onSuccess: (next) => queryClient.setQueryData(screenKey('people'), next),
  })
}

function Mention({ m }: { m: MentionRow }) {
  return (
    <div className="space-y-1">
      <div className="text-sm text-neutral-500">
        {[m.role, m.date, m.group, m.author].filter(Boolean).join(' · ')}
        {m.link && <a className="ml-2 underline" href={m.link} target="_blank" rel="noreferrer">t.me</a>}
      </div>
      <div className="whitespace-pre-wrap">{m.text}</div>
    </div>
  )
}

function Card({ card }: { card: PersonCard }) {
  return (
    <div className={`${boxClass} space-y-3`}>
      <h2 className="text-lg font-semibold">{card.name}</h2>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        {card.fields.map((f) => [<dt key={`${f.key}-l`} className="text-neutral-500">{f.label}</dt>, <dd key={f.key}>{f.value}</dd>])}
      </dl>
      {(card.mentions ?? []).map((m, i) => <div key={i} className="border-t border-neutral-200 pt-2 dark:border-neutral-800"><Mention m={m} /></div>)}
    </div>
  )
}

function Candidate({ row }: { row: CandidateRow }) {
  const merge = usePeopleAction<{ a: number; b: number }>('merge')
  const reject = usePeopleAction<{ a: number; b: number }>('reject')
  const busy = merge.isPending || reject.isPending
  const run = (id: string) => (id === 'merge' ? merge : reject).mutate({ a: row.a.id, b: row.b.id })
  return (
    <div className={`${boxClass} flex items-center gap-4`}>
      {[row.a, row.b].map((p) => (
        <div key={p.id} className="space-y-0.5">
          <div className="font-medium">{p.name}</div>
          {p.lines.map((l) => <div key={l} className="text-sm text-neutral-500">{l}</div>)}
        </div>
      ))}
      <div className="ml-auto flex gap-2">
        {row.actions.map((a) => (
          <Button key={a.id} variant={a.tone === 'primary' ? 'default' : 'outline'} disabled={busy} onClick={() => run(a.id)}>{a.label}</Button>
        ))}
      </div>
    </div>
  )
}

function Unlinked({ row }: { row: UnlinkedRow }) {
  const options = row.options ?? []
  const [person, setPerson] = useState(options[0]?.id ?? -1)
  const link = usePeopleAction<{ mention: number; person: number }>('link')
  return (
    <div className={`${boxClass} space-y-2`}>
      <Mention m={row.mention} />
      {options.length > 0 && (
        <div className="flex items-center gap-2">
          <select className={inputClass} value={person} onChange={(e) => setPerson(Number(e.target.value))}>
            {options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}
          </select>
          <Button disabled={link.isPending} onClick={() => link.mutate({ mention: row.id, person })}>{row.link_label}</Button>
        </div>
      )}
    </div>
  )
}

export function PeoplePage() {
  const [view, setView] = useState<View>({ tab: '', q: '', id: 0 })
  const [search, setSearch] = useState('')
  const current = useRef(view)
  current.current = view
  const query = useQuery({ queryKey: screenKey('people'), queryFn: () => getJSON<PeopleScreen>(screenURL(current.current)) })
  const { refetch } = query
  useEffect(() => {
    void refetch()
  }, [view, refetch])
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-3">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          <div className="flex gap-2">
            {s.tabs.map((t) => (
              <Button key={t.id} variant={t.id === s.tab ? 'default' : 'ghost'} onClick={() => setView({ tab: t.id, q: view.q, id: 0 })}>
                {t.label}
              </Button>
            ))}
          </div>
          {s.message && <p className="text-neutral-600 dark:text-neutral-300">{s.message}</p>}
          {s.tab === '' && (
            <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); setView({ tab: '', q: search, id: 0 }) }}>
              <input className={`${inputClass} w-80`} placeholder={s.search_label} value={search} onChange={(e) => setSearch(e.target.value)} />
            </form>
          )}
          {s.card && <Card card={s.card} />}
          {s.tab === '' && (s.rows ?? []).map((row) => (
            <div key={row.id} className={`${boxClass} flex items-center gap-3`}>
              <div className="space-y-0.5">
                <div className="font-medium">{row.title}</div>
                {row.lines.map((l) => <div key={l} className="text-sm text-neutral-500">{l}</div>)}
              </div>
              <Button className="ml-auto" variant="outline" onClick={() => setView({ tab: '', q: view.q, id: row.id })}>{s.open_label}</Button>
            </div>
          ))}
          {s.tab === 'candidates' && (s.candidates ?? []).map((row) => <Candidate key={`${row.a.id}-${row.b.id}`} row={row} />)}
          {s.tab === 'unlinked' && (s.unlinked ?? []).map((row) => <Unlinked key={row.id} row={row} />)}
          {s.empty_text && <p className="text-neutral-500">{s.empty_text}</p>}
        </section>
      )}
    </ScreenView>
  )
}
