import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import postcss from 'postcss'
import tailwindcss from 'tailwindcss'
import theme from '../../tailwind.config.js'

describe('运行时动效样式', () => {
  it('即使静态模板没有出现 Vue 过渡类，构建仍保留完整进入和退出规则', async () => {
    const path = resolve(__dirname, '../style.css')
    // 输入使用空 div，检查 Tailwind 是否保留运行时过渡类。
    const result = await postcss([tailwindcss({ ...theme, content: [{ raw: '<div></div>' }] })])
      .process(readFileSync(path, 'utf8'), { from: path })
    for (const name of ['fade', 'fade-slow', 'dropdown-fade', 'pop-float', 'pop-fade', 'modal', 'collapse', 'motion-list']) {
      for (const phase of ['enter-active', 'enter-from', 'leave-active', 'leave-to']) {
        expect(result.css).toContain(`.${name}-${phase}`)
      }
    }
    expect(result.css).toContain('grid-template-rows: 0fr')
    expect(result.css).toContain('var(--motion-layout)')
    expect(result.css).toContain('@media (prefers-reduced-motion: reduce)')
  })
})
