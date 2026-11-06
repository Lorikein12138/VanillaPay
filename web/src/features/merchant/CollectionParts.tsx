import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { moduleError } from './collection-data'

export function ModuleError({ error, retry }: { error: unknown; retry?: () => void }) {
  const { t } = useTranslation()
  return <Notice tone="error" action={retry && <Button onClick={retry}>{t('error.retry')}</Button>}>{t(moduleError(error), { defaultValue: moduleError(error) })}</Notice>
}
// Local async actions never put security proofs or returned keys in mutation caches.
export function useLocalAction() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>()
  const active = useRef(true), running = useRef(false)
  useEffect(() => { active.current = true; return () => { active.current = false } }, [])
  async function run(action: () => Promise<void>) {
    if (running.current) return
    running.current = true; setBusy(true); setError(undefined)
    try { await action() } catch (problem) { if (active.current) setError(problem) }
    finally { running.current = false; if (active.current) setBusy(false) }
  }
  return { busy, error, run, active }
}
