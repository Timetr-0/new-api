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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Dialog } from '@/components/dialog'
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

const rateLimitSpecRegex =
  /^N\(\s*(?:(?:mean|mu)\s*=\s*)?\d+(?:\.\d+)?\s*,\s*(?:(?:std|stddev|sigma)\s*=\s*)?\d+(?:\.\d+)?\s*\)$/i

const isValidRateLimitSpec = (value: string, allowZero: boolean) => {
  const text = value.trim()
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

const rateLimitDialogSchema = z.object({
  groupName: z.string().min(1, 'Group name is required'),
  maxRequests: z.string().refine((value) => isValidRateLimitSpec(value, true), {
    message: 'Must be ≥ 0',
  }),
  maxSuccess: z.string().refine((value) => isValidRateLimitSpec(value, false), {
    message: 'Must be ≥ 1',
  }),
})

type RateLimitDialogFormValues = z.infer<typeof rateLimitDialogSchema>

const RATE_LIMIT_FORM_ID = 'rate-limit-form'

export type RateLimitEntryData = {
  groupName: string
  maxRequests: string
  maxSuccess: string
}

type RateLimitDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: RateLimitEntryData) => void
  editData?: RateLimitEntryData | null
}

export function RateLimitDialog({
  open,
  onOpenChange,
  onSave,
  editData,
}: RateLimitDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData

  const form = useForm<RateLimitDialogFormValues>({
    resolver: zodResolver(rateLimitDialogSchema),
    defaultValues: {
      groupName: '',
      maxRequests: '0',
      maxSuccess: '1',
    },
  })

  useEffect(() => {
    if (editData) {
      form.reset(editData)
    } else {
      form.reset({
        groupName: '',
        maxRequests: '0',
        maxSuccess: '1',
      })
    }
  }, [editData, form, open])

  const handleSubmit = (values: RateLimitDialogFormValues) => {
    onSave(values)
    form.reset()
    onOpenChange(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        isEditMode ? t('Edit group rate limit') : t('Add group rate limit')
      }
      description={t(
        'Configure rate limiting rules for a specific user group.'
      )}
      contentClassName='sm:max-w-[500px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={RATE_LIMIT_FORM_ID}>
            {isEditMode ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={RATE_LIMIT_FORM_ID}
          onSubmit={form.handleSubmit(handleSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='groupName'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Group Name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('e.g., default, vip, premium')}
                    {...field}
                    disabled={isEditMode}
                  />
                </FormControl>
                <FormDescription>
                  {isEditMode
                    ? t('Group name cannot be changed when editing.')
                    : t('Unique identifier for this group.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='maxRequests'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Max Requests (including failures)')}</FormLabel>
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
                  {t('Total requests allowed per period. 0 = unlimited.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='maxSuccess'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Max Successful Requests')}</FormLabel>
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
                  {t('Only successful requests count toward this limit.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
