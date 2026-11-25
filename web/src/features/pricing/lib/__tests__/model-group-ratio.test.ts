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
import { describe, expect, it } from 'vitest'

import { FILTER_ALL } from '../../constants'
import type { PricingModel } from '../../types'
import {
  getDisplayGroupRatio,
  getModelEffectiveGroupRatio,
  getModelGroupRatio,
} from '../model-helpers'

const MODEL_GROUP_RATIO = {
  default: { 'gpt-4': 0.25, '*': 0.9 },
  vip: { '*': 0.5 },
}

function pricingModel(overrides: Partial<PricingModel>): PricingModel {
  return {
    id: 1,
    model_name: 'gpt-4',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default', 'vip'],
    ...overrides,
  }
}

describe('model group ratio overrides', () => {
  it('prefers the exact model name over the wildcard', () => {
    expect(getModelGroupRatio(MODEL_GROUP_RATIO, 'default', 'gpt-4')).toBe(0.25)
    expect(getModelGroupRatio(MODEL_GROUP_RATIO, 'default', 'gpt-5')).toBe(0.9)
  })

  it('returns undefined when the group or the model has no override', () => {
    expect(
      getModelGroupRatio(MODEL_GROUP_RATIO, 'svip', 'gpt-4')
    ).toBeUndefined()
    expect(
      getModelGroupRatio({ svip: { 'other-model': 0.3 } }, 'svip', 'gpt-4')
    ).toBeUndefined()
    expect(getModelGroupRatio(undefined, 'default', 'gpt-4')).toBeUndefined()
  })

  it('ignores a non-finite ratio so the group ratio still applies', () => {
    expect(
      getModelGroupRatio(
        { default: { 'gpt-4': '0.2' as unknown as number } },
        'default',
        'gpt-4'
      )
    ).toBeUndefined()
  })

  it('falls back to the configured group ratio without an override', () => {
    const model = pricingModel({ model_group_ratio: MODEL_GROUP_RATIO })
    expect(getModelEffectiveGroupRatio(model, 'default', { default: 2 })).toBe(
      0.25
    )
    expect(getModelEffectiveGroupRatio(model, 'svip', { svip: 3 })).toBe(3)
    expect(getModelEffectiveGroupRatio(model, 'svip', {})).toBe(1)
  })

  it('keeps a zero override, which means the model is free in that group', () => {
    const model = pricingModel({
      model_group_ratio: { default: { 'gpt-4': 0 } },
    })
    expect(getModelEffectiveGroupRatio(model, 'default', { default: 2 })).toBe(
      0
    )
  })

  it('uses the override of the selected group for the summary price', () => {
    const model = pricingModel({
      group_ratio: { default: 2, vip: 1 },
      model_group_ratio: MODEL_GROUP_RATIO,
    })
    expect(getDisplayGroupRatio(model, 'default')).toBe(0.25)
    expect(getDisplayGroupRatio(model, 'vip')).toBe(0.5)
    // Without a group filter the cheapest available ratio is shown.
    expect(getDisplayGroupRatio(model, FILTER_ALL)).toBe(0.25)
  })

  it('considers overrides from every enabled group when no group is selected', () => {
    const model = pricingModel({
      group_ratio: { default: 2, vip: 1 },
      model_group_ratio: { vip: { 'gpt-4': 0.2 } },
    })
    expect(getDisplayGroupRatio(model)).toBe(0.2)
  })

  it('skips enabled groups without a configured or overridden ratio', () => {
    const model = pricingModel({
      enable_groups: ['default', 'vip'],
      group_ratio: { vip: 1.5 },
    })
    expect(getDisplayGroupRatio(model)).toBe(1.5)
    expect(getDisplayGroupRatio(pricingModel({ group_ratio: {} }))).toBe(1)
  })
})
