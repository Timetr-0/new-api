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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'

import { updateModelAPIExample, type ModelAPIExample } from '../api'

export function ModelAPIExampleEditor(props: {
  example: ModelAPIExample
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [code, setCode] = useState(props.example.code)
  const save = useMutation({
    mutationFn: updateModelAPIExample,
    onSuccess: async (_data, example) => {
      await queryClient.invalidateQueries({
        queryKey: ['model-api-examples', example.model_name],
      })
      toast.success(t('Code sample saved'))
      props.onOpenChange(false)
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to save code sample'))
    },
  })

  return (
    <Dialog open onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{t('Edit code sample')}</DialogTitle>
          <DialogDescription>
            {t(
              'Code samples are visible to users. Use placeholders for API keys.'
            )}
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor='model-api-example-code'>
              {props.example.model_name} · {props.example.endpoint_type} ·{' '}
              {props.example.language}
            </FieldLabel>
            <Textarea
              id='model-api-example-code'
              value={code}
              onChange={(event) => setCode(event.target.value)}
              rows={18}
              maxLength={60 * 1024}
              disabled={save.isPending}
              spellCheck={false}
              className='max-h-[55vh] font-mono text-xs'
            />
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button
            variant='outline'
            disabled={save.isPending}
            onClick={() => save.mutate({ ...props.example, code: '' })}
          >
            {t('Restore generated sample')}
          </Button>
          <Button
            variant='outline'
            disabled={save.isPending}
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            disabled={save.isPending}
            onClick={() => save.mutate({ ...props.example, code })}
          >
            {save.isPending ? t('Saving...') : t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
