/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { Braces, RotateCcw } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../../components/settings-form-layout'
import { SettingsPageFormActions } from '../../components/settings-page-context'
import { SettingsSection } from '../../components/settings-section'
import { useUpdateOption } from '../../hooks/use-update-option'
import { DEFAULT_REQUEST_INTERCEPTION_RULES_TEXT } from './constants'

const rulesJson = z.string().refine((value) => {
  const trimmed = value.trim()
  if (!trimmed) return true
  try {
    return Array.isArray(JSON.parse(trimmed))
  } catch {
    return false
  }
}, 'Rules JSON must be an array')

const schema = z.object({
  request_interception_setting: z.object({
    enabled: z.boolean(),
    rules: rulesJson,
  }),
})

type RequestInterceptionFormValues = z.output<typeof schema>
type RequestInterceptionFormInput = z.input<typeof schema>

type RequestInterceptionSettings = {
  request_interception_setting: {
    enabled: boolean
    rules: string
  }
}

type FlatRequestInterceptionSettings = {
  'request_interception_setting.enabled': boolean
  'request_interception_setting.rules': string
}

function normalizeRulesText(value: string) {
  const trimmed = (value ?? '').toString().trim()
  if (!trimmed) return '[]'
  try {
    return JSON.stringify(JSON.parse(trimmed))
  } catch {
    return trimmed
  }
}

function flattenRequestInterceptionValues(
  values: RequestInterceptionFormValues
): FlatRequestInterceptionSettings {
  return {
    'request_interception_setting.enabled':
      values.request_interception_setting.enabled,
    'request_interception_setting.rules': normalizeRulesText(
      values.request_interception_setting.rules
    ),
  }
}

type Props = {
  defaultValues: RequestInterceptionSettings
}

export function RequestInterceptionSection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<
    RequestInterceptionFormInput,
    unknown,
    RequestInterceptionFormValues
  >({
    resolver: zodResolver(schema),
    defaultValues: props.defaultValues as RequestInterceptionFormInput,
  })

  useEffect(() => {
    form.reset(props.defaultValues as RequestInterceptionFormInput)
  }, [form, props.defaultValues])

  const formatRulesJson = () => {
    const raw = form.getValues('request_interception_setting.rules')
    if (!raw || !raw.trim()) return
    try {
      const formatted = JSON.stringify(JSON.parse(raw), null, 2)
      form.setValue('request_interception_setting.rules', formatted, {
        shouldDirty: true,
      })
    } catch {
      toast.error(t('Invalid JSON format'))
    }
  }

  const restoreDefaultRules = () => {
    form.setValue(
      'request_interception_setting.rules',
      DEFAULT_REQUEST_INTERCEPTION_RULES_TEXT,
      { shouldDirty: true }
    )
    toast.success(t('Default rule restored'))
  }

  const onSubmit = async (values: RequestInterceptionFormValues) => {
    const flattenedDefaults = flattenRequestInterceptionValues(
      props.defaultValues
    )
    const flattenedValues = flattenRequestInterceptionValues(values)
    const updates = Object.entries(flattenedValues).filter(
      ([key, value]) =>
        value !==
        flattenedDefaults[key as keyof FlatRequestInterceptionSettings]
    )

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({
        key,
        value,
      })
    }
  }

  return (
    <SettingsSection title={t('Request Interception')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />

          <Alert>
            <AlertDescription className='text-xs'>
              {t(
                'Rules run against the final upstream JSON after conversion and parameter overrides. The first matching rule returns its configured error response locally.'
              )}
            </AlertDescription>
          </Alert>

          <FormField
            control={form.control}
            name='request_interception_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Request Interception')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Reject matching requests before they are sent to upstream providers.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='request_interception_setting.rules'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Rules JSON')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={18}
                    spellCheck={false}
                    className='font-mono text-xs'
                    placeholder={DEFAULT_REQUEST_INTERCEPTION_RULES_TEXT}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Use gjson paths in conditions. Leave the array empty to keep interception enabled without active rules.'
                  )}
                </FormDescription>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={formatRulesJson}
                  >
                    <Braces data-icon='inline-start' />
                    <span>{t('Format JSON')}</span>
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={restoreDefaultRules}
                  >
                    <RotateCcw data-icon='inline-start' />
                    <span>{t('Restore Default Rule')}</span>
                  </Button>
                </div>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
