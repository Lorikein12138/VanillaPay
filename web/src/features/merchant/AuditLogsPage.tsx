import { auditQuery } from '@/app/portal-queries'
import { useResetPortalCursors } from '@/app/portal-cursors'
import { DateTimePicker } from '@/components/ui/date-time-picker'
import { localDateInput, parseLocalDateTime } from '@/lib/date-time'
import { Select, SelectOption } from '@/components/ui/select'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { CopyText } from '@/components/ui/copy-button'
import { Dialog } from '@/components/ui/dialog'
import { DetailList } from '@/components/ui/detail-list'
import { Field, FieldGrid, FormActions } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { BackLink, Card, EmptyState, Mono, Page, PageHeader, Skeleton } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { RecordLink } from '@/components/ui/records'
import { Table, Td, Th } from '@/components/ui/table'
import { auditActions, auditMerchantId, auditRoles, type AuditFilters } from './audit-data'
import { ModuleError } from './CollectionParts'
import type { Schema } from './collection-data'
import { OrderTime } from './OrderParts'

type FilterProps = { filters: AuditFilters; onFilters: (value: AuditFilters) => void; isConsole?: boolean }

function AuditMerchant({ id }: { id: Schema['PortalAuditDTO']['merchant_id'] }) {
  return id ? (
    <RecordLink to={`/console/merchants/${encodeURIComponent(id)}`}>
      <Mono>{id}</Mono>
    </RecordLink>
  ) : (
    '—'
  )
}

export function AuditLogsPage({
  name,
  filters,
  onFilters,
  role = 'merchant',
}: Omit<FilterProps, 'isConsole'> & {
  name: string
  role?: 'merchant' | 'console'
}) {
  const { t } = useTranslation(),
    [selected, setSelected] = useState<Schema['PortalAuditDTO'] | null>(null)
  const isConsole = role === 'console'
  const title = t(isConsole ? 'audit.consoleTitle' : 'audit.title')
  const options = auditQuery(role, name, filters)
  const query = useInfiniteQuery(options)
  useResetPortalCursors(options.queryKey)
  const items = query.data?.pages.flatMap((page) => page.items ?? []) ?? []
  const actionName = (action: string) => t(`audit.actions.${action}`, { defaultValue: action })
  const result = (value: string) => (
    <StatusDot tone={value === 'success' ? 'ok' : 'off'}>
      {t(`audit.results.${value}`, { defaultValue: value })}
    </StatusDot>
  )
  return (
    <Page>
      <PageHeader
        title={title}
        description={t(isConsole ? 'audit.consoleDescription' : 'audit.description')}
        back={isConsole ? undefined : <BackLink to="/account">{t('polish.accountSecurity')}</BackLink>}
      />
      <AuditFiltersForm key={JSON.stringify(filters)} filters={filters} onFilters={onFilters} isConsole={isConsole} />
      {query.isPending && <Skeleton className="h-64" />}
      {query.isError && (
        <ModuleError
          error={query.error}
          retry={() => {
            void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch())
          }}
        />
      )}
      {query.data &&
        (items.length ? (
          <>
            <Table caption={title} minWidth={800} className="hidden md:block">
              <thead>
                <tr>
                  <Th>{t('audit.time')}</Th>
                  <Th>{t('audit.action')}</Th>
                  {isConsole && <Th>{t('audit.merchant')}</Th>}
                  <Th>{t('audit.result')}</Th>
                  <Th>{t('audit.ip')}</Th>
                  <Th>{t('audit.request')}</Th>
                  <Th>{t('audit.details')}</Th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id}>
                    <Td>
                      <OrderTime value={item.created_at} />
                    </Td>
                    <Td>{actionName(item.action)}</Td>
                    {isConsole && (
                      <Td>
                        <AuditMerchant id={item.merchant_id} />
                      </Td>
                    )}
                    <Td>{result(item.result)}</Td>
                    <Td>
                      <Mono>{item.ip ?? '—'}</Mono>
                    </Td>
                    <Td className="max-w-64">
                      <CopyText value={item.request_id} label={t('audit.copyRequest')} />
                    </Td>
                    <Td>
                      <Button variant="ghost" size="sm" onClick={() => setSelected(item)}>
                        {t('audit.details')}
                      </Button>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <ul aria-label={t('audit.mobileList')} className="m-0 flex list-none flex-col gap-3 p-0 md:hidden">
              {items.map((item) => (
                <li key={item.id}>
                  <Card>
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <p className="m-0 text-label font-medium">{actionName(item.action)}</p>
                      {result(item.result)}
                    </div>
                    <p className="mb-4 mt-2 text-caption text-text-3">
                      <OrderTime value={item.created_at} />
                    </p>
                    <DetailList
                      columns={2}
                      items={[
                        ...(isConsole
                          ? [{ label: t('audit.merchant'), value: <AuditMerchant id={item.merchant_id} /> }]
                          : []),
                        { label: t('audit.ip'), value: <Mono>{item.ip ?? '—'}</Mono> },
                        {
                          label: t('audit.request'),
                          value: <CopyText value={item.request_id} label={t('audit.copyRequest')} />,
                        },
                      ]}
                    />
                    <div className="mt-3">
                      <Button variant="secondary" size="sm" onClick={() => setSelected(item)}>
                        {t('audit.details')}
                      </Button>
                    </div>
                  </Card>
                </li>
              ))}
            </ul>
          </>
        ) : (
          <EmptyState title={t('audit.empty')} />
        ))}
      {query.hasNextPage && !query.isPlaceholderData && (
        <div className="flex justify-center">
          <Button
            loading={query.isFetchingNextPage}
            onClick={() => {
              void query.fetchNextPage()
            }}
          >
            {t('polish.loadMore')}
          </Button>
        </div>
      )}
      {selected && <AuditDetails item={selected} onClose={() => setSelected(null)} isConsole={isConsole} />}
    </Page>
  )
}
export function AuditDetails({
  item,
  onClose,
  isConsole,
}: {
  item: Schema['PortalAuditDTO']
  onClose: () => void
  isConsole?: boolean
}) {
  const { t } = useTranslation()
  const actor = t(`audit.roles.${item.actor_role}`, { defaultValue: item.actor_role })
  return (
    <Dialog title={t('audit.detailTitle')} size="lg" onClose={onClose}>
      <DetailList
        columns={2}
        items={[
          { label: t('audit.action'), value: t(`audit.actions.${item.action}`, { defaultValue: item.action }) },
          {
            label: t('audit.result'),
            value: (
              <StatusDot tone={item.result === 'success' ? 'ok' : 'off'}>
                {t(`audit.results.${item.result}`, { defaultValue: item.result })}
              </StatusDot>
            ),
          },
          {
            label: t('audit.actor'),
            value:
              isConsole && item.actor_id ? (
                <>
                  {actor} · <Mono>{item.actor_id}</Mono>
                </>
              ) : (
                actor
              ),
          },
          ...(isConsole ? [{ label: t('audit.merchant'), value: <AuditMerchant id={item.merchant_id} /> }] : []),
          {
            label: t('audit.target'),
            value: (
              <>
                {t(`audit.targets.${item.target_type}`, { defaultValue: item.target_type })} ·{' '}
                <Mono>{item.target_id}</Mono>
              </>
            ),
          },
          { label: t('audit.time'), value: <OrderTime value={item.created_at} /> },
          { label: t('audit.ip'), value: item.ip ?? '—', mono: true },
          { label: t('audit.request'), value: <CopyText value={item.request_id} label={t('audit.copyRequest')} />, wide: true },
        ]}
      />
      {item.safe_metadata && Object.keys(item.safe_metadata).length > 0 && (
        <div className="border-t-[0.5px] border-border pt-3.5">
          <p className="mb-1 mt-0 text-caption text-text-3">{t('audit.metadata')}</p>
          <pre className="m-0 whitespace-pre-wrap rounded-control bg-muted p-4">
            <Mono>{JSON.stringify(item.safe_metadata, null, 2)}</Mono>
          </pre>
        </div>
      )}
    </Dialog>
  )
}
function AuditFiltersForm({ filters, onFilters, isConsole }: FilterProps) {
  const { t } = useTranslation(),
    id = useId()
  const [action, setAction] = useState(filters.action ?? ''),
    [result, setResult] = useState(filters.result ?? '')
  const [start, setStart] = useState(localDateInput(filters.start)),
    [end, setEnd] = useState(localDateInput(filters.end)),
    [invalid, setInvalid] = useState(false)
  const [merchantId, setMerchantId] = useState(filters.merchant_id ?? '')
  const [actorRole, setActorRole] = useState<NonNullable<AuditFilters['actor_role']> | ''>(filters.actor_role ?? '')
  const parsedId = auditMerchantId(merchantId.trim())
  const invalidId = !!merchantId.trim() && !parsedId
  return (
    <Card>
      <form
        aria-label={t('audit.filters')}
        className="flex flex-col gap-5"
        onSubmit={(event) => {
          event.preventDefault()
          if (
            (start && Number.isNaN(parseLocalDateTime(start))) ||
            (end && Number.isNaN(parseLocalDateTime(end))) ||
            (start && end && parseLocalDateTime(start) >= parseLocalDateTime(end))
          ) {
            setInvalid(true)
            return
          }
          setInvalid(false)
          if (invalidId) return
          onFilters({
            action: action || undefined,
            result: result || undefined,
            start: start ? new Date(start).toISOString() : undefined,
            end: end ? new Date(end).toISOString() : undefined,
            ...(isConsole && { merchant_id: parsedId, actor_role: actorRole || undefined }),
          })
        }}
      >
        <FieldGrid
          className={
            isConsole ? 'grid-cols-1 sm:grid-cols-2 xl:grid-cols-3' : 'grid-cols-1 sm:grid-cols-2 xl:grid-cols-4'
          }
        >
          <Field label={t('audit.action')} htmlFor={id + '-action'}>
            <Select id={id + '-action'} value={action} onValueChange={setAction}>
              <SelectOption value="">{t('audit.allActions')}</SelectOption>
              {action && !auditActions.some((value) => value === action) && <SelectOption value={action}>{action}</SelectOption>}
              {auditActions.map((value) => (
                <SelectOption key={value} value={value}>
                  {t(`audit.actions.${value}`, { defaultValue: value })}
                </SelectOption>
              ))}
            </Select>
          </Field>
          <Field label={t('audit.result')} htmlFor={id + '-result'}>
            <Select id={id + '-result'} value={result} onValueChange={setResult}>
              <SelectOption value="">{t('audit.allResults')}</SelectOption>
              <SelectOption value="success">{t('audit.results.success')}</SelectOption>
              <SelectOption value="failure">{t('audit.results.failure')}</SelectOption>
            </Select>
          </Field>
          {isConsole && (
            <>
              <Field label={t('audit.actorRole')} htmlFor={id + '-role'}>
                <Select
                  id={id + '-role'}
                  value={actorRole}
                  onValueChange={(next) => {
                    setActorRole(auditRoles.find((role) => role === next) ?? '')
                  }}
                >
                  <SelectOption value="">{t('audit.allRoles')}</SelectOption>
                  {auditRoles.map((role) => (
                    <SelectOption key={role} value={role}>
                      {t(`audit.roles.${role}`)}
                    </SelectOption>
                  ))}
                </Select>
              </Field>
              <Field
                label={t('audit.merchantId')}
                htmlFor={id + '-merchant'}
                error={invalidId && t('audit.invalidMerchantId')}
              >
                <Input
                  id={id + '-merchant'}
                  value={merchantId}
                  inputMode="numeric"
                  aria-invalid={invalidId || undefined}
                  aria-describedby={invalidId ? id + '-merchant-hint' : undefined}
                  onChange={(event) => setMerchantId(event.target.value)}
                />
              </Field>
            </>
          )}
          <Field label={t('polish.startTime')} htmlFor={id + '-start'}>
            <DateTimePicker
              id={id + '-start'}
              value={start}
              onValueChange={setStart}
            />
          </Field>
          <Field label={t('polish.endTime')} htmlFor={id + '-end'}>
            <DateTimePicker
              id={id + '-end'}
              value={end}
              onValueChange={setEnd}
            />
          </Field>
        </FieldGrid>
        {invalid && <Notice tone="error">{t('polish.enterValidDatesTheEndMustBe')}</Notice>}
        <FormActions className="border-t-[0.5px] border-border pt-5">
          <Button
            variant="ghost"
            onClick={() => {
              setAction('')
              setResult('')
              setStart('')
              setEnd('')
              setInvalid(false)
              setMerchantId('')
              setActorRole('')
              onFilters({})
            }}
          >
            {t('polish.reset')}
          </Button>
          <Button type="submit" variant="primary">
            {t('polish.applyFilters')}
          </Button>
        </FormActions>
      </form>
    </Card>
  )
}
