import { describe, expect, it } from 'vitest'

import {
  KEY_REDIRECT_RULES,
  WILDCARD_ONLY_RULES,
  firstModelMappingIssue,
  mappingRowsToRecord,
  recordToMappingRows,
  validateModelMappingRows,
} from '../modelMappingRules'

describe('validateModelMappingRows', () => {
  it('未开启任何检查时不报告问题', () => {
    expect(validateModelMappingRows([{ from: '', to: 'a*b' }])).toEqual([{}])
  })

  it('按必填、长度、通配符、重复的顺序检查来源', () => {
    const tooLong = 'a'.repeat(101)
    const issues = validateModelMappingRows(
      [
        { from: '  ', to: 'x' },
        { from: tooLong, to: 'x' },
        { from: 'a*b', to: 'x' },
        { from: 'dup', to: 'x' },
        { from: ' dup ', to: 'y' },
      ],
      KEY_REDIRECT_RULES,
    )

    expect(issues.map((issue) => issue.from)).toEqual([
      'sourceRequired',
      'nameTooLong',
      'sourceWildcardInvalid',
      'duplicateSource',
      'duplicateSource',
    ])
  })

  it('按必填、长度、通配符、自映射的顺序检查目标', () => {
    const issues = validateModelMappingRows(
      [
        { from: 'a', to: '' },
        { from: 'b', to: 'x'.repeat(101) },
        { from: 'c', to: 'gpt-*' },
        { from: 'd', to: ' d ' },
      ],
      KEY_REDIRECT_RULES,
    )

    expect(issues.map((issue) => issue.to)).toEqual([
      'targetRequired',
      'nameTooLong',
      'targetWildcardInvalid',
      'selfMapping',
    ])
  })

  it('长度按码点计算', () => {
    const emoji = '😀'.repeat(100)
    expect(validateModelMappingRows([{ from: emoji, to: 'x' }], KEY_REDIRECT_RULES)).toEqual([{}])
  })

  it('允许单独的末尾通配符', () => {
    expect(validateModelMappingRows([{ from: '*', to: 'x' }], KEY_REDIRECT_RULES)).toEqual([{}])
    expect(validateModelMappingRows([{ from: 'gpt-**', to: 'x' }], KEY_REDIRECT_RULES)[0].from)
      .toBe('sourceWildcardInvalid')
  })

  it('仅通配符规则可以同时报告来源和目标', () => {
    expect(validateModelMappingRows([{ from: 'a*b', to: 'c*' }], WILDCARD_ONLY_RULES)).toEqual([
      { from: 'sourceWildcardInvalid', to: 'targetWildcardInvalid' },
    ])
    expect(validateModelMappingRows([{ from: '', to: '' }], WILDCARD_ONLY_RULES)).toEqual([{}])
  })
})

describe('firstModelMappingIssue', () => {
  it('按行序返回第一个问题并优先报告来源', () => {
    expect(firstModelMappingIssue([{}, { to: 'selfMapping', from: 'duplicateSource' }]))
      .toBe('duplicateSource')
    expect(firstModelMappingIssue([{}, {}])).toBeNull()
  })
})

describe('映射对象转换', () => {
  it('提交时去掉首尾空白', () => {
    expect(mappingRowsToRecord([{ from: ' a ', to: ' b ' }])).toEqual({ a: 'b' })
  })

  it('展开时保持键顺序并兼容空值', () => {
    expect(recordToMappingRows({ b: '1', a: '2' })).toEqual([
      { from: 'b', to: '1' },
      { from: 'a', to: '2' },
    ])
    expect(recordToMappingRows(null)).toEqual([])
  })
})
