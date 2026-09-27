// Файл: web/src/pages/JobsPage.tsx
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { postJSON } from '../api'
import { ScreenView } from '../components/ScreenView'
import { Button } from '../components/ui/button'
import { screenKey, useScreen } from '../queries'
import type { JobRow, JobsScreen } from '../types'

const toneClass: Record<JobRow['status_tone'], string> = {
  neutral: 'text-neutral-500',
  running: 'text-blue-600',
  warning: 'text-amber-600',
  success: 'text-green-600',
  danger: 'text-red-600',
}

const buttonVariant = { primary: 'default', danger: 'danger', default: 'outline' } as const

export function JobsPage() {
  const query = useScreen<JobsScreen>('jobs')
  const queryClient = useQueryClient()
  const action = useMutation({
    mutationFn: (v: { id: number; action: string }) => postJSON<JobsScreen>(`/api/jobs/${v.id}/${v.action}`, {}),
    onSuccess: (screen) => queryClient.setQueryData(screenKey('jobs'), screen),
  })
  return (
    <ScreenView query={query}>
      {(s) => (
        <section className="space-y-3">
          <h1 className="text-xl font-semibold">{s.title}</h1>
          {action.error && <p className="text-red-600">{action.error.message}</p>}
          {s.jobs.length === 0 && <p className="text-neutral-500">{s.empty_text}</p>}
          {s.jobs.map((job) => (
            <div key={job.id} className="space-y-1 rounded-md border border-neutral-200 p-3 dark:border-neutral-800">
              <div className="flex items-center gap-2">
                <span className="font-medium">{job.title}</span>
                <span className={toneClass[job.status_tone]}>{job.status_text}</span>
                <span className="ml-auto text-sm text-neutral-500">{job.updated_text}</span>
              </div>
              {job.progress_text && <p className="text-sm">{job.progress_text}</p>}
              {job.reason && <p className="text-sm text-amber-700 dark:text-amber-400">{job.reason}</p>}
              <div className="flex gap-2">
                {job.actions.map((a) => (
                  <Button key={a.id} variant={buttonVariant[a.tone]} disabled={action.isPending}
                    onClick={() => action.mutate({ id: job.id, action: a.id })}>
                    {a.label}
                  </Button>
                ))}
              </div>
            </div>
          ))}
        </section>
      )}
    </ScreenView>
  )
}
