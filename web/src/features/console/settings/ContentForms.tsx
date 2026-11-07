import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox, Select, Textarea } from '@/components/ui/input'
import { Field, FieldGrid } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { SectionHeader } from '@/components/ui/layout'
import { AccountField } from '@/features/auth/account-shared'
import { newFooterItem, type Schema as S } from '../settings-data'

export function HomeForm({
  value,
  onChange,
  errors,
}: {
  value: S['Home']
  onChange: (value: S['Home']) => void
  errors: Record<string, string | undefined>
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-5">
      <Field label={t('consoleSettings.homeMode')} htmlFor="home-mode" error={errors.mode}>
        <Select
          id="home-mode"
          value={value.mode}
          onChange={(e) => onChange({ mode: e.target.value as S['Home']['mode'], html: null, url: null })}
        >
          <option value="default">{t('consoleSettings.homeDefault')}</option>
          <option value="html">HTML</option>
          <option value="url">URL</option>
        </Select>
      </Field>
      {value.mode === 'html' && (
        <Field label="HTML" htmlFor="home-html" hint={t('consoleSettings.homeHtmlHint')} error={errors.html}>
          <Textarea
            id="home-html"
            rows={10}
            maxLength={262144}
            value={value.html ?? ''}
            onChange={(e) => onChange({ ...value, html: e.target.value, url: null })}
          />
        </Field>
      )}
      {value.mode === 'url' && (
        <AccountField
          label={t('consoleSettings.homeUrl')}
          type="url"
          value={value.url ?? ''}
          maxLength={2048}
          error={errors.url}
          onChange={(e) => onChange({ ...value, url: e.target.value, html: null })}
        />
      )}
    </div>
  )
}

export function FooterForm({
  value,
  onChange,
  errors,
  errorText,
}: {
  value: S['SettingsFooterLegal']
  onChange: (value: S['SettingsFooterLegal']) => void
  errors: Record<string, string>
  errorText: (key: string) => string | undefined
}) {
  const { t } = useTranslation()
  const items = value.items ?? []
  function replaceItem(index: number, patch: Partial<S['SettingsFooterItem']>) {
    onChange({ ...value, items: items.map((item, i) => (i === index ? { ...item, ...patch } : item)) })
  }
  return (
    <div className="space-y-6">
      <div className="space-y-4">
        {items.map((item, index) => (
          <div key={item.id} className="space-y-4 border-b-[0.5px] border-border pb-5">
            <FieldGrid>
              <AccountField
                label={t('consoleSettings.footerZh')}
                value={item.label.zh_CN}
                error={errorText(`item_${index}`)}
                onChange={(e) => replaceItem(index, { label: { ...item.label, zh_CN: e.target.value } })}
              />
              <AccountField
                label={t('consoleSettings.footerEn')}
                value={item.label.en}
                error={errorText(`item_${index}`)}
                onChange={(e) => replaceItem(index, { label: { ...item.label, en: e.target.value } })}
              />
              <AccountField
                label={t('consoleSettings.footerText')}
                value={item.text}
                error={errorText(`item_${index}`)}
                onChange={(e) => replaceItem(index, { text: e.target.value })}
              />
              <AccountField
                label={t('consoleSettings.footerUrl')}
                value={item.url ?? ''}
                error={errorText(`item_url_${index}`)}
                onChange={(e) => replaceItem(index, { url: e.target.value || null })}
              />
            </FieldGrid>
            <div className="flex flex-wrap items-center gap-2">
              <Checkbox
                checked={item.enabled}
                onChange={(e) => replaceItem(index, { enabled: e.target.checked })}
              >
                {t('consoleSettings.enabled')}
              </Checkbox>
              <Button
                size="sm"
                disabled={index === 0}
                onClick={() => {
                  const next = [...items]
                  const previous = next[index - 1]!
                  next[index - 1] = next[index]!
                  next[index] = previous
                  onChange({ ...value, items: next })
                }}
              >
                {t('consoleSettings.moveUp')}
              </Button>
              <Button
                size="sm"
                disabled={index === items.length - 1}
                onClick={() => {
                  const next = [...items]
                  const following = next[index + 1]!
                  next[index + 1] = next[index]!
                  next[index] = following
                  onChange({ ...value, items: next })
                }}
              >
                {t('consoleSettings.moveDown')}
              </Button>
              <Button
                size="sm"
                variant="dangerOutline"
                onClick={() => onChange({ ...value, items: items.filter((_, i) => i !== index) })}
              >
                {t('consoleSettings.remove')}
              </Button>
            </div>
          </div>
        ))}
      </div>
      {errors.items && <p className="m-0 text-caption text-danger">{errorText('items')}</p>}
      <Button
        disabled={items.length >= 50}
        onClick={() => onChange({ ...value, items: [...items, newFooterItem()] })}
      >
        {t('consoleSettings.addFooter')}
      </Button>
      <Notice tone="info">{t('consoleSettings.legalVersionNotice')}</Notice>
      {(['terms', 'privacy'] as const).map((kind) => (
        <div key={kind} className="space-y-3">
          <SectionHeader as="h3" title={t(`consoleSettings.${kind}`)} />
          <Field label={t('consoleSettings.zhContent')} htmlFor={`${kind}-zh`} error={errorText(kind)}>
            <Textarea
              id={`${kind}-zh`}
              rows={6}
              value={value[kind].zh_CN}
              onChange={(e) => onChange({ ...value, [kind]: { ...value[kind], zh_CN: e.target.value } })}
            />
          </Field>
          <Field label={t('consoleSettings.enContent')} htmlFor={`${kind}-en`}>
            <Textarea
              id={`${kind}-en`}
              rows={6}
              value={value[kind].en}
              onChange={(e) => onChange({ ...value, [kind]: { ...value[kind], en: e.target.value } })}
            />
          </Field>
        </div>
      ))}
    </div>
  )
}
