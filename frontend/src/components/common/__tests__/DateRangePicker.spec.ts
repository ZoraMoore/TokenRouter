import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

const messages: Record<string, string> = {
  'dates.today': 'Today',
  'dates.yesterday': 'Yesterday',
  'dates.last15Minutes': 'Last 15 Minutes',
  'dates.last30Minutes': 'Last 30 Minutes',
  'dates.last24Hours': 'Last 24 Hours',
  'dates.last7Days': 'Last 7 Days',
  'dates.last14Days': 'Last 14 Days',
  'dates.last30Days': 'Last 30 Days',
  'dates.thisMonth': 'This Month',
  'dates.lastMonth': 'Last Month',
  'dates.startDate': 'Start Date',
  'dates.endDate': 'End Date',
  'dates.apply': 'Apply',
  'dates.selectDateRange': 'Select date range',
  'dates.selectEndDate': 'Select an end date',
  'dates.previousMonth': 'Previous month',
  'dates.nextMonth': 'Next month',
  'common.cancel': 'Cancel'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en')
  })
}))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const waitForTransition = () => new Promise((resolve) => window.setTimeout(resolve, 250))

describe('DateRangePicker', () => {
  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: formatLocalDate(yesterday),
        endDate: formatLocalDate(now)
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 24 Hours')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')
    await wrapper.find('.date-picker-actions .btn-primary').trigger('click')

    const nowAfterClick = new Date()
    const yesterdayAfterClick = new Date(nowAfterClick.getTime() - 24 * 60 * 60 * 1000)
    const expectedStart = formatLocalDate(yesterdayAfterClick)
    const expectedEnd = formatLocalDate(nowAfterClick)

    expect(wrapper.emitted('update:startDate')?.[0]).toEqual([expectedStart])
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual([expectedEnd])
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: expectedStart,
        endDate: expectedEnd,
        preset: 'last24Hours'
      }
    ])
  })

  it('can apply a preset immediately when applyOnPreset is enabled', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today,
        applyOnPreset: true
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last Month')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')

    expect(wrapper.emitted('change')?.[0]?.[0]).toMatchObject({
      preset: 'lastMonth'
    })
    await waitForTransition()
    expect(wrapper.find('.date-picker-dropdown').exists()).toBe(false)
  })

  it('selects a custom range from the calendar and swaps reversed clicks', async () => {
    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: '2026-01-10',
        endDate: '2026-01-12'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.input-trigger').trigger('click')
    const dayButton = (day: number) =>
      wrapper.findAll('.date-picker-day').find((node) => node.text() === String(day))!

    // 先点 20 日再点 5 日，结束日期早于开始日期时自动对调。
    await dayButton(20).trigger('click')
    expect(wrapper.find('.date-picker-actions .btn-primary').attributes('disabled')).toBeDefined()
    await dayButton(5).trigger('click')
    expect(wrapper.findAll('.date-picker-day-in-range')).toHaveLength(16)

    await wrapper.find('.date-picker-actions .btn-primary').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: '2026-01-05',
        endDate: '2026-01-20',
        preset: null
      }
    ])
  })

  it('discards unapplied changes when cancelled', async () => {
    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: '2026-01-10',
        endDate: '2026-01-12'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    const triggerText = wrapper.find('.input-trigger').text()
    await wrapper.find('.input-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 7 Days')
    )
    await presetButton!.trigger('click')
    // 未应用前触发器仍显示原范围。
    expect(wrapper.find('.input-trigger').text()).toBe(triggerText)
    expect(wrapper.find('.date-picker-preset-active').text()).toContain('Last 7 Days')

    const cancelButton = wrapper.findAll('.date-picker-actions .btn').find((node) =>
      node.text().includes('Cancel')
    )
    await cancelButton!.trigger('click')

    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.find('.input-trigger').text()).toBe(triggerText)
  })
})
