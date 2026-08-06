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
import { Code2, Palette } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

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
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { RateLimitVisualEditor } from './rate-limit-visual-editor'

const isValidJSON = (value: string | undefined) => {
  if (!value || value.trim() === '') return true
  try {
    const parsed = JSON.parse(value)
    if (typeof parsed !== 'object' || Array.isArray(parsed)) {
      return false
    }
    for (const [, val] of Object.entries(parsed)) {
      if (!Array.isArray(val) || val.length !== 2) return false
      if (!isValidRateLimitSpec(val[0], true)) return false
      if (!isValidRateLimitSpec(val[1], false)) return false
    }
    return true
  } catch {
    return false
  }
}

const rateLimitSpecRegex =
  /^N\(\s*(?:(?:mean|mu)\s*=\s*)?\d+(?:\.\d+)?\s*,\s*(?:(?:std|stddev|sigma)\s*=\s*)?\d+(?:\.\d+)?\s*\)$/i

const isValidRateLimitSpec = (value: unknown, allowZero: boolean) => {
  const text =
    typeof value === 'number'
      ? String(value)
      : typeof value === 'string'
        ? value.trim()
        : ''
  if (text === '') return false

  const parsed = Number(text)
  if (Number.isInteger(parsed)) {
    return parsed <= 2147483647 && (allowZero ? parsed >= 0 : parsed >= 1)
  }

  if (!rateLimitSpecRegex.test(text)) return false
  const [, body] = text.match(/^N\((.*)\)$/i) ?? []
  if (!body) return false
  const values = body.split(',').map((part) => part.trim())
  const meanText = values[0].includes('=')
    ? values[0].split('=')[1]?.trim()
    : values[0]
  const stdText = values[1]?.includes('=')
    ? values[1].split('=')[1]?.trim()
    : values[1]
  const mean = Number(meanText)
  const std = Number(stdText)
  return (
    Number.isFinite(mean) &&
    Number.isFinite(std) &&
    mean >= 1 &&
    mean <= 2147483647 &&
    std >= 0 &&
    std <= 2147483647
  )
}

const createRateLimitSchema = (t: (key: string) => string) =>
  z.object({
    GlobalApiRateLimitEnabled: z.boolean(),
    GlobalApiRateLimitNum: z
      .string()
      .refine((value) => isValidRateLimitSpec(value, false), {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
    GlobalApiRateLimitDuration: z.number().min(1).max(2147483647),
    ModelRequestRateLimitEnabled: z.boolean(),
    ModelRequestRateLimitDurationMinutes: z.number().min(0),
    ModelRequestRateLimitCount: z
      .string()
      .refine((value) => isValidRateLimitSpec(value, true), {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
    ModelRequestRateLimitSuccessCount: z
      .string()
      .refine((value) => isValidRateLimitSpec(value, false), {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
    ModelRequestRateLimitGroup: z
      .string()
      .optional()
      .refine(isValidJSON, {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
    AwsBedrockRateLimitEnabled: z.boolean(),
    AwsBedrockRateLimitCount: z
      .string()
      .refine((value) => isValidRateLimitSpec(value, true), {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
    AwsBedrockRateLimitPerSecondCount: z.number().min(0).max(2147483647),
    AwsBedrockRateLimitQueueTimeoutSeconds: z
      .number()
      .min(1)
      .max(2147483647),
    AwsBedrockRateLimitQueueMaxSize: z.number().min(0).max(2147483647),
  })

type RateLimitFormValues = z.infer<ReturnType<typeof createRateLimitSchema>>

type RateLimitSectionProps = {
  defaultValues: RateLimitFormValues
}

export function RateLimitSection({ defaultValues }: RateLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [useVisualEditor, setUseVisualEditor] = useState(true)

  const rateLimitSchema = createRateLimitSchema(t)

  const form = useForm<RateLimitFormValues>({
    resolver: zodResolver(rateLimitSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: RateLimitFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof RateLimitFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Rate Limiting')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save rate limits'
          />
          <div className='space-y-1'>
            <h3 className='text-sm font-semibold'>
              {t('Global API rate limiting')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Applies to all /api routes and takes effect immediately after saving.'
              )}
            </p>
          </div>
          <FormField
            control={form.control}
            name='GlobalApiRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
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

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='GlobalApiRateLimitDuration'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Limit period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={1}
                        max={2147483647}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 1)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('seconds')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Time window for rate limiting')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='GlobalApiRateLimitNum'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max requests per period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='text'
                        placeholder='N(700,std=100)'
                        {...field}
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Including failed requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='space-y-1'>
            <h3 className='text-sm font-semibold'>
              {t('Model request rate limiting')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t('Applies to authenticated model relay requests.')}
            </p>
          </div>
          <FormField
            control={form.control}
            name='ModelRequestRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
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

          <div className='grid gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='ModelRequestRateLimitDurationMinutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Limit period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('minutes')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Time window for rate limiting')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max requests per period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='text'
                        placeholder='N(700,std=100)'
                        {...field}
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Including failed requests, 0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitSuccessCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max successful requests')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='text'
                        placeholder='N(700,std=100)'
                        {...field}
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Only successful requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='ModelRequestRateLimitGroup'
            render={({ field }) => (
              <FormItem>
                <div className='flex items-center justify-between'>
                  <FormLabel>{t('Group-based rate limits')}</FormLabel>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => setUseVisualEditor(!useVisualEditor)}
                  >
                    {useVisualEditor ? (
                      <>
                        <Code2 className='mr-2 h-4 w-4' />
                        {t('JSON Mode')}
                      </>
                    ) : (
                      <>
                        <Palette className='mr-2 h-4 w-4' />
                        {t('Visual Mode')}
                      </>
                    )}
                  </Button>
                </div>
                <FormControl>
                  {useVisualEditor ? (
                    <RateLimitVisualEditor
                      value={field.value || ''}
                      onChange={field.onChange}
                    />
                  ) : (
                    <Textarea
                      rows={8}
                      placeholder={`{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}`}
                      className='font-mono text-sm'
                      {...field}
                    />
                  )}
                </FormControl>
                {!useVisualEditor && (
                  <FormDescription>
                    <div className='space-y-1 text-xs'>
                      <p className='font-semibold'>{t('Format:')}</p>
                      <ul className='list-inside list-disc space-y-0.5 pl-2'>
                        <li>
                          {t('JSON object:')}{' '}
                          {`{"groupName": [maxRequests, maxSuccess]}`}
                        </li>
                        <li>
                          {t('Example:')}{' '}
                          {`{"default": [200, 100], "vip": [0, 1000]}`}
                        </li>
                        <li>
                          {t(
                            'maxRequests ≥ 0, maxSuccess ≥ 1, both ≤ 2,147,483,647'
                          )}
                        </li>
                        <li>
                          {t(
                            'Group config overrides global limits, shares the same period'
                          )}
                        </li>
                      </ul>
                    </div>
                  </FormDescription>
                )}
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='space-y-1'>
            <h3 className='text-sm font-semibold'>
              {t('AWS Bedrock upstream rate limiting')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Applies before sending requests to AWS Bedrock. Excess requests wait in a synchronous queue.'
              )}
            </p>
          </div>
          <FormField
            control={form.control}
            name='AwsBedrockRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
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

          <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
            <FormField
              control={form.control}
              name='AwsBedrockRateLimitCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Requests per minute')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='text'
                        placeholder='N(2000,std=10)'
                        {...field}
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Sampled once per minute when using N(mean,std=stddev), 0 = unlimited'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AwsBedrockRateLimitPerSecondCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Requests per second')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        max={2147483647}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Controls short-burst speed before the per-minute limit, 0 = unlimited'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AwsBedrockRateLimitQueueTimeoutSeconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max wait time')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={1}
                        max={2147483647}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 1)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('seconds')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('How long a request can wait for an upstream send slot')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AwsBedrockRateLimitQueueMaxSize'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max queued requests')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={2147483647}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(Number.parseInt(e.target.value) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Maximum waiting requests, 0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
