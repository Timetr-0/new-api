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
import { api } from '@/lib/api'

import type { PricingData } from './types'

// ----------------------------------------------------------------------------
// Pricing APIs
// ----------------------------------------------------------------------------

// Get model pricing data
export async function getPricing(): Promise<PricingData> {
  const res = await api.get('/api/pricing')
  return res.data
}

export type ModelAPIExample = {
  model_name: string
  endpoint_type: string
  language: 'curl' | 'python' | 'typescript' | 'javascript'
  code: string
}

export async function getModelAPIExamples(model: string) {
  const res = await api.get<{
    success: boolean
    message?: string
    data: Record<string, Partial<Record<ModelAPIExample['language'], string>>>
  }>('/api/model-api-examples', { params: { model } })
  if (!res.data.success) throw new Error(res.data.message)
  return res.data.data
}

export async function updateModelAPIExample(example: ModelAPIExample) {
  const res = await api.put<{ success: boolean; message?: string }>(
    '/api/models/api_example',
    example
  )
  if (!res.data.success) throw new Error(res.data.message)
}
