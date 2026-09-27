// Файл: web/src/App.tsx
import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { postJSON } from './api'
import { ScreenView } from './components/ScreenView'
import { Button } from './components/ui/button'
import { useServerEvents } from './events'
import { HomePage } from './pages/HomePage'
import { JobsPage } from './pages/JobsPage'
import { SettingsPage } from './pages/SettingsPage'
import { TelegramPage } from './pages/TelegramPage'
import { FormsPage } from './pages/FormsPage'
import { PeoplePage } from './pages/PeoplePage'
import { TablePage } from './pages/TablePage'
import { useScreen } from './queries'
import { toggleTheme } from './theme'
import type { Layout } from './types'

const pages: Record<string, () => JSX.Element> = {
  home: HomePage,
  telegram: TelegramPage,
  people: PeoplePage,
  table: TablePage,
  forms: FormsPage,
  jobs: JobsPage,
  settings: SettingsPage,
}

export function App() {
  const [closedText, setClosedText] = useState('')
  useServerEvents(closedText === '')
  const layout = useScreen<Layout>('layout')
  const [page, setPage] = useState('home')
  const quit = useMutation({
    mutationFn: (text: string) => postJSON<unknown>('/api/quit', {}).then(() => text),
    onSuccess: (text) => {
      setClosedText(text)
      window.close() // окно программы (--app) закроется; обычная вкладка останется с текстом
    },
  })
  const Page = pages[page] ?? HomePage
  if (closedText !== '') return <p className="p-6">{closedText}</p>
  return (
    <div className="min-h-screen bg-white text-neutral-900 dark:bg-neutral-950 dark:text-neutral-100">
      <ScreenView query={layout}>
        {(l) => (
          <header className="flex items-center gap-2 border-b border-neutral-200 px-4 py-2 dark:border-neutral-800">
            <span className="mr-2 font-semibold">{l.app_name}</span>
            {l.nav.map((item) => (
              <Button key={item.id} variant={item.id === page ? 'default' : 'ghost'} onClick={() => setPage(item.id)}>
                {item.label}
              </Button>
            ))}
            <div className="ml-auto flex gap-2">
              <Button variant="ghost" onClick={toggleTheme}>{l.theme_label}</Button>
              <Button variant="outline" disabled={quit.isPending} onClick={() => quit.mutate(l.closed_text)}>{l.quit_label}</Button>
            </div>
          </header>
        )}
      </ScreenView>
      <main className="mx-auto max-w-5xl p-4">
        <Page />
      </main>
    </div>
  )
}
