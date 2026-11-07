import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { FieldGrid, FormActions } from '@/components/ui/field'
import { Card, Page, PageHeader } from '@/components/ui/layout'
import { FormHint, FormStack } from '@/components/ui/auth-screen'
import { RecordLink, RecordResults, Records, RecordText } from '@/components/ui/records'
import { AccountError, AccountField, validPassword, validUsername } from '@/features/auth/account-shared'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { OrderTime } from '@/features/merchant/OrderParts'
import { Choice, DeviceStatus, MerchantStatus } from './ManageParts'
import { invalidateConsole, planName, useCursorList, type MerchantFilters } from './manage-data'

export function MerchantsPage({ filters, onFilters }: { filters: MerchantFilters; onFilters: (filters: MerchantFilters) => void }) {
  const { t, i18n } = useTranslation(), [create, setCreate] = useState(false)
  const query = useCursorList(['console', 'merchants', filters], (cursor, signal) => consoleApi.merchants({ ...filters, cursor, limit: 20 }, signal))
  const items = query.data?.pages.flatMap((page) => page.items ?? []) ?? []
  return <Page>
    <PageHeader title={t('manage.merchants')} description={query.data && t('manage.merchantTotal', { count: query.data.pages[0]?.total })}
      actions={<Button variant="primary" onClick={() => setCreate(true)}>{t('manage.createMerchant')}</Button>} />
    <MerchantFiltersForm key={JSON.stringify(filters)} filters={filters} onFilters={onFilters} />
    <RecordResults busy={query.isPending} error={query.error} retry={() => void query.refetch()}
      more={query.hasNextPage} loadingMore={query.isFetchingNextPage} onMore={() => void query.fetchNextPage()} empty={!!query.data && !items.length}>
      <Records items={items} id={(item) => item.id} label={t('manage.merchants')} columns={[
        { label: t('manage.merchant'), value: (item) => <><RecordText>{item.username}</RecordText><RecordText secondary>{item.email} · {item.pid}</RecordText></> },
        { label: t('manage.plan'), value: (item) => planName(item.subscription.plan, i18n.language) || t('manage.none') },
        { label: t('manage.expiry'), value: (item) => <OrderTime value={item.subscription.expires_at} /> },
        { label: t('manage.device'), value: (item) => <DeviceStatus device={item.device} /> },
        { label: t('manage.status'), value: (item) => <MerchantStatus banned={item.banned} /> },
        { label: t('manage.manage'), value: (item) => <RecordLink to={'/console/merchants/' + item.id}>{t('manage.manage')}</RecordLink> },
      ]} />
    </RecordResults>
    {create && <CreateMerchant onClose={() => setCreate(false)} />}
  </Page>
}
function MerchantFiltersForm({ filters, onFilters }: { filters: MerchantFilters; onFilters: (value: MerchantFilters) => void }) {
  const { t } = useTranslation(), [query, setQuery] = useState(filters.query ?? '')
  const [banned, setBanned] = useState(filters.banned === undefined ? '' : String(filters.banned))
  const [subscription, setSubscription] = useState(filters.subscription_active === undefined ? '' : String(filters.subscription_active))
  return <Card><FormStack onSubmit={(event) => {
    event.preventDefault()
    if (Array.from(query.trim()).length <= 100) onFilters({ query: query.trim() || undefined, banned: banned === '' ? undefined : banned === 'true', subscription_active: subscription === '' ? undefined : subscription === 'true' })
  }}><FieldGrid>
    <AccountField label={t('manage.search')} value={query} onChange={(event) => setQuery(event.target.value)} />
    <Choice label={t('manage.status')} value={banned} onChange={setBanned}>
      <option value="">{t('manage.all')}</option><option value="false">{t('manage.normal')}</option><option value="true">{t('manage.banned')}</option>
    </Choice>
    <Choice label={t('manage.subscription')} value={subscription} onChange={setSubscription}>
      <option value="">{t('manage.all')}</option><option value="true">{t('manage.active')}</option><option value="false">{t('manage.inactive')}</option>
    </Choice>
  </FieldGrid><FormActions>
    <Button onClick={() => { setQuery(''); setBanned(''); setSubscription(''); onFilters({}) }}>{t('manage.resetFilters')}</Button>
    <Button type="submit" variant="primary" disabled={Array.from(query.trim()).length > 100}>{t('manage.apply')}</Button>
  </FormActions></FormStack></Card>
}
function CreateMerchant({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation(), action = useLocalAction(), client = useQueryClient(), navigate = useNavigate()
  const [username, setUsername] = useState(''), [email, setEmail] = useState('')
  const [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('')
  const [fieldError, setFieldError] = useState<'' | 'USERNAME_TAKEN' | 'EMAIL_TAKEN'>('')
  const valid = validUsername(username) && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()) && validPassword(password, confirmation)
  async function submit() {
    if (!valid) return
    await action.run(async () => {
      setFieldError('')
      try {
        const user = await consoleApi.createMerchant({ username: username.trim(), email: email.trim(), password, password_confirmation: confirmation })
        await invalidateConsole(client)
        if (!action.active.current) return
        setPassword(''); setConfirmation('')
        if (action.active.current) { onClose(); await navigate({ to: '/console/merchants/$id', params: { id: user.id } }) }
      } catch (error) {
        if (!action.active.current) return
        if (error instanceof APIError && (error.problem?.code === 'USERNAME_TAKEN' || error.problem?.code === 'EMAIL_TAKEN')) setFieldError(error.problem.code)
        else throw error
      }
    })
  }
  return <Dialog title={t('manage.createMerchant')} onClose={onClose} busy={action.busy}>
    <FormStack onSubmit={(event) => { event.preventDefault(); void submit() }}>
      <FormHint>{t('manage.validation')}</FormHint>
      <AccountField label={t('manage.username')} value={username} autoComplete="off" disabled={action.busy}
        error={fieldError === 'USERNAME_TAKEN' && t('manage.USERNAME_TAKEN')} onChange={(event) => { setUsername(event.target.value); setFieldError('') }} />
      <AccountField label={t('manage.email')} value={email} type="email" disabled={action.busy}
        error={fieldError === 'EMAIL_TAKEN' && t('manage.EMAIL_TAKEN')} onChange={(event) => { setEmail(event.target.value); setFieldError('') }} />
      <AccountField label={t('manage.password')} value={password} type="password" autoComplete="new-password" disabled={action.busy} onChange={(event) => setPassword(event.target.value)} />
      <AccountField label={t('manage.confirmation')} value={confirmation} type="password" autoComplete="new-password" disabled={action.busy} onChange={(event) => setConfirmation(event.target.value)} />
      {action.error != null && <AccountError error={action.error} />}
      <FormActions><Button onClick={onClose} disabled={action.busy}>{t('manage.cancel')}</Button>
        <Button type="submit" variant="primary" loading={action.busy} disabled={!valid}>{t('manage.createMerchant')}</Button></FormActions>
    </FormStack>
  </Dialog>
}
