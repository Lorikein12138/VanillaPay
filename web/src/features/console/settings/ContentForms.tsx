import { Select, SelectOption } from '@/components/ui/select'
import { useTranslation } from 'react-i18next'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox, Textarea } from '@/components/ui/input'
import { Field, FieldGrid } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { Card, SubsectionTitle } from '@/components/ui/layout'
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
          onValueChange={(next) => onChange({ mode: next as S['Home']['mode'], html: null, url: null })}
        >
          <SelectOption value="default">{t('consoleSettings.homeDefault')}</SelectOption>
          <SelectOption value="html">HTML</SelectOption>
          <SelectOption value="url">URL</SelectOption>
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
      <div className="flex flex-col gap-3">
        <SubsectionTitle>{t('consoleSettings.footerLinks')}</SubsectionTitle>
        {items.length === 0 && <p className="m-0 text-label text-text-3">{t('consoleSettings.footerEmpty')}</p>}
        {items.map((item, index) => (
          <Card key={item.id} className="space-y-3 p-3 sm:p-4">
            <div className="flex items-center justify-between gap-2">
              <p className="m-0 text-label font-medium">{t('consoleSettings.footerItemNumber', { number: index + 1 })}</p>
              <div className="flex items-center">
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t('consoleSettings.moveUp')}
                  title={t('consoleSettings.moveUp')}
                  disabled={index === 0}
                  onClick={() => {
                    const next = [...items]
                    const previous = next[index - 1]!
                    next[index - 1] = next[index]!
                    next[index] = previous
                    onChange({ ...value, items: next })
                  }}
                >
                  <ArrowUp aria-hidden="true" />
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t('consoleSettings.moveDown')}
                  title={t('consoleSettings.moveDown')}
                  disabled={index === items.length - 1}
                  onClick={() => {
                    const next = [...items]
                    const following = next[index + 1]!
                    next[index + 1] = next[index]!
                    next[index] = following
                    onChange({ ...value, items: next })
                  }}
                >
                  <ArrowDown aria-hidden="true" />
                </Button>
                <Button
                  size="icon"
                  variant="danger"
                  aria-label={t('consoleSettings.remove')}
                  title={t('consoleSettings.remove')}
                  onClick={() => onChange({ ...value, items: items.filter((_, i) => i !== index) })}
                >
                  <Trash2 aria-hidden="true" />
                </Button>
              </div>
            </div>
            <FieldGrid className="grid-cols-1 sm:grid-cols-2">
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
              {(['zh_CN', 'en'] as const).map((language) => {
                const id = `footer-text-${index}-${language}`
                const error = errorText(`item_${index}`)
                return (
                  <Field
                    key={language}
                    label={t(language === 'zh_CN' ? 'consoleSettings.footerTextZh' : 'consoleSettings.footerTextEn')}
                    htmlFor={id}
                    error={error}
                  >
                    <Textarea
                      id={id}
                      rows={3}
                      value={item.text[language]}
                      aria-invalid={!!error}
                      aria-describedby={error ? `${id}-hint` : undefined}
                      onChange={(e) => replaceItem(index, { text: { ...item.text, [language]: e.target.value } })}
                    />
                  </Field>
                )
              })}
              <AccountField
                label={t('consoleSettings.footerUrl')}
                value={item.url ?? ''}
                error={errorText(`item_url_${index}`)}
                onChange={(e) => replaceItem(index, { url: e.target.value || null })}
              />
            </FieldGrid>
            <Checkbox checked={item.enabled} onChange={(e) => replaceItem(index, { enabled: e.target.checked })}>
              {t('consoleSettings.enabled')}
            </Checkbox>
          </Card>
        ))}
        {errors.items && <p className="m-0 text-caption text-danger">{errorText('items')}</p>}
        <Button
          className="self-start"
          disabled={items.length >= 50}
          onClick={() => onChange({ ...value, items: [...items, newFooterItem()] })}
        >
          {t('consoleSettings.addFooter')}
        </Button>
      </div>
      <Notice tone="info">{t('consoleSettings.legalVersionNotice')}</Notice>
      {(['terms', 'privacy'] as const).map((kind) => (
        <div key={kind} className="space-y-3">
          <SubsectionTitle className="mb-5">{t(`consoleSettings.${kind}`)}</SubsectionTitle>
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
