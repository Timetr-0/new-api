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

export const DEFAULT_REQUEST_INTERCEPTION_RULES = [
  {
    name: 'aws claude thinking with forced tool choice',
    channel_types: [33],
    relay_formats: ['claude'],
    model_regex: ['^claude-.*'],
    path_regex: ['/v1/messages', '/v1/chat/completions', '/v1/responses'],
    methods: ['POST'],
    conditions: [
      {
        path: 'thinking.type',
        operator: 'exists',
      },
      {
        path: 'tool_choice.type',
        operator: 'in',
        values: ['any', 'tool'],
      },
    ],
    error: {
      status_code: 400,
      type: 'invalid_request_error',
      code: 'invalid_request',
      message:
        'ValidationException: Thinking may not be enabled when tool_choice forces tool use.',
    },
  },
] as const

export const DEFAULT_REQUEST_INTERCEPTION_RULES_TEXT = JSON.stringify(
  DEFAULT_REQUEST_INTERCEPTION_RULES,
  null,
  2
)
