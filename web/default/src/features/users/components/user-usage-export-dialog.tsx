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
import { Download, Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { exportUserUsage } from '../api'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const EXPORT_FIELDS = [
  { key: 'user_id', label: 'User ID' },
  { key: 'username', label: 'Username' },
  { key: 'model_name', label: 'Model' },
  { key: 'cost_usd', label: 'Cost (USD)' },
  { key: 'tokens', label: 'Tokens' },
  { key: 'requests', label: 'Requests' },
  { key: 'quota', label: 'Quota' },
] as const

const DEFAULT_FIELDS = new Set([
  'user_id',
  'username',
  'model_name',
  'cost_usd',
  'tokens',
])

function toDatetimeLocalValue(date: Date) {
  const offset = date.getTimezoneOffset()
  return new Date(date.getTime() - offset * 60 * 1000)
    .toISOString()
    .slice(0, 16)
}

function getDefaultRange() {
  const end = new Date()
  const start = new Date(end)
  start.setHours(0, 0, 0, 0)
  return {
    start: toDatetimeLocalValue(start),
    end: toDatetimeLocalValue(end),
  }
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

function toFilenamePart(value: string) {
  return value.replace(/[^\d]/g, '')
}

export function UserUsageExportDialog({ open, onOpenChange }: Props) {
  const { t } = useTranslation()
  const defaultRange = useMemo(getDefaultRange, [])
  const [startTime, setStartTime] = useState(defaultRange.start)
  const [endTime, setEndTime] = useState(defaultRange.end)
  const [selectedFields, setSelectedFields] = useState<string[]>(
    EXPORT_FIELDS.filter((field) => DEFAULT_FIELDS.has(field.key)).map(
      (field) => field.key
    )
  )
  const [exporting, setExporting] = useState(false)

  const toggleField = (field: string, checked: boolean) => {
    setSelectedFields((current) => {
      if (checked) return current.includes(field) ? current : [...current, field]
      return current.filter((item) => item !== field)
    })
  }

  const handleExport = async () => {
    if (selectedFields.length === 0) {
      toast.error(t('Select at least one export field'))
      return
    }
    const startTimestamp = Math.floor(new Date(startTime).getTime() / 1000)
    const endTimestamp = Math.floor(new Date(endTime).getTime() / 1000)
    if (!startTimestamp || !endTimestamp || endTimestamp <= startTimestamp) {
      toast.error(t('End time must be later than start time'))
      return
    }

    setExporting(true)
    try {
      const blob = await exportUserUsage({
        start_timestamp: startTimestamp,
        end_timestamp: endTimestamp,
        fields: selectedFields,
      })
      downloadBlob(
        blob,
        `user_usage_${toFilenamePart(startTime)}_${toFilenamePart(endTime)}.csv`
      )
      onOpenChange(false)
    } catch {
      toast.error(t('Failed to export usage'))
    } finally {
      setExporting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Export user usage')}</DialogTitle>
          <DialogDescription>
            {t('Choose fields and a time range before exporting.')}
          </DialogDescription>
        </DialogHeader>

        <div className='grid gap-4'>
          <div className='grid gap-2 sm:grid-cols-2'>
            <div className='grid gap-2'>
              <Label htmlFor='user-usage-export-start'>{t('Start time')}</Label>
              <Input
                id='user-usage-export-start'
                type='datetime-local'
                value={startTime}
                onChange={(event) => setStartTime(event.target.value)}
              />
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='user-usage-export-end'>{t('End time')}</Label>
              <Input
                id='user-usage-export-end'
                type='datetime-local'
                value={endTime}
                onChange={(event) => setEndTime(event.target.value)}
              />
            </div>
          </div>

          <div className='grid gap-3'>
            <Label>{t('Export fields')}</Label>
            <div className='grid gap-3 sm:grid-cols-2'>
              {EXPORT_FIELDS.map((field) => (
                <Label
                  key={field.key}
                  className='flex items-center gap-2 font-normal'
                >
                  <Checkbox
                    checked={selectedFields.includes(field.key)}
                    onCheckedChange={(checked) =>
                      toggleField(field.key, !!checked)
                    }
                  />
                  {t(field.label)}
                </Label>
              ))}
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleExport} disabled={exporting}>
            {exporting ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <Download className='h-4 w-4' />
            )}
            {t('Export')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
