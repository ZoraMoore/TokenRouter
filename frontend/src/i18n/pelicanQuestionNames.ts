import type { PelicanPlanKind } from '@/api/pelicanTests'

export const pelicanBuiltinNames = {
  en: { pelican: 'Pelican SVG', candy: 'Candy puzzle' },
  'zh-Hans': { pelican: '鹈鹕 SVG', candy: '糖果题' },
}

// 已保存的内置题名和早期空题名都使用界面语言。自定义题或改过的题名按原文显示。
export function overriddenQuestionName(question: { question_kind: PelicanPlanKind; question_name?: string }): string | undefined {
  const { question_kind: kind, question_name: name } = question
  if (kind === 'pelican' || kind === 'candy') {
    if (name === pelicanBuiltinNames['zh-Hans'][kind] || name === pelicanBuiltinNames.en[kind]) return undefined
  }
  return name || undefined
}
