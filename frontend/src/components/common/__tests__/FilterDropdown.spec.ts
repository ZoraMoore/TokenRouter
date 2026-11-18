import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'

import FilterDropdown from '../FilterDropdown.vue'
import FilterField from '../FilterField.vue'
import Select from '../Select.vue'
import { BREAKPOINT_LG } from '@/constants/layout'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const stubs = { Icon: true, MotionTransition: { template: '<div><slot /></div>' } }

// 把触发区放在指定横坐标，模拟按钮位于页面左侧或右侧。
const openAt = async (left: number, columns: 1 | 2 | 3) => {
  const wrapper = mount(FilterDropdown, {
    props: { activeCount: 0, columns },
    global: { stubs },
  })
  vi.spyOn(wrapper.element, 'getBoundingClientRect').mockReturnValue({ left, right: left + 36, top: 0, bottom: 36, width: 36 } as DOMRect)
  await wrapper.get('button').trigger('click')
  return wrapper.get('.filter-panel')
}

describe('FilterDropdown', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('右侧放得下时面板左缘对齐触发按钮', async () => {
    window.innerWidth = BREAKPOINT_LG
    const panel = await openAt(100, 3)
    expect(panel.attributes('style')).toContain('left: 0px')
    expect(panel.attributes('style')).toContain('width: 768px')
  })

  it('右侧放不下时面板向左平移并留出视口边距', async () => {
    window.innerWidth = BREAKPOINT_LG
    const panel = await openAt(900, 2)
    // 34rem = 544px，右缘停在 1024 - 16 = 1008px，相对触发器左移 900 - 464 = 436px。
    expect(panel.attributes('style')).toContain('left: -436px')
  })

  it('按 columns 切换栅格列数', async () => {
    window.innerWidth = BREAKPOINT_LG
    const panel = await openAt(100, 3)
    expect(panel.get('.filter-panel-body').classes()).toEqual(expect.arrayContaining(['lg:grid-cols-3']))
  })

  it('没有生效条件时禁用重置，有条件时触发 reset', async () => {
    const wrapper = mount(FilterDropdown, { props: { activeCount: 0 }, global: { stubs } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.get('[data-testid="filter-reset"]').attributes('disabled')).toBeDefined()

    await wrapper.setProps({ activeCount: 2 })
    expect(wrapper.get('button').text()).toBe('2')
    await wrapper.get('[data-testid="filter-reset"]').trigger('click')
    expect(wrapper.emitted('reset')).toHaveLength(1)
  })

  it('面板顶部按字段顺序列出已选条件，移除标签会恢复该字段的默认值', async () => {
    const Host = defineComponent({
      components: { FilterDropdown, FilterField, AppSelect: Select },
      setup() {
        const brand = ref('openai')
        const status = ref('')
        const keyword = ref('gpt')
        const range = ref('6h')
        const options = [{ value: '', label: '全部' }, { value: 'openai', label: 'OpenAI' }]
        const rangeOptions = [{ value: '1h', label: '1 小时' }, { value: '6h', label: '6 小时' }]
        return { brand, status, keyword, range, options, rangeOptions }
      },
      template: `
        <FilterDropdown :active-count="3">
          <FilterField label="关键词" :value-text="keyword" @clear="keyword = ''">
            <input v-model="keyword" />
          </FilterField>
          <FilterField label="品牌"><AppSelect v-model="brand" :options="options" /></FilterField>
          <FilterField label="状态"><AppSelect v-model="status" :options="options" /></FilterField>
          <FilterField label="时间" empty-value="1h"><AppSelect v-model="range" :options="rangeOptions" /></FilterField>
        </FilterDropdown>
      `,
    })
    const wrapper = mount(Host, { global: { stubs } })
    await wrapper.get('button[aria-haspopup="dialog"]').trigger('click')

    const chipTexts = () => wrapper.findAll('.filter-chip').map((chip) => chip.text())
    expect(chipTexts()).toEqual(['关键词gpt', '品牌OpenAI', '时间6 小时'])

    await wrapper.findAll('.filter-chip-remove')[1]!.trigger('click')
    expect((wrapper.vm as unknown as { brand: string }).brand).toBe('')
    await wrapper.findAll('.filter-chip-remove')[1]!.trigger('click')
    expect((wrapper.vm as unknown as { range: string }).range).toBe('1h')
    await wrapper.findAll('.filter-chip-remove')[0]!.trigger('click')
    expect(wrapper.find('[data-testid="filter-chips"]').exists()).toBe(false)
  })
})
