import { expect, it } from 'vitest'
import {
  cleanDanmakuMessages,
  danmakuMessageLengthError,
  validateDanmakuMessage,
} from './danmakuMessage'

it.each([
  { character: '中', limit: 40 },
  { character: '。', limit: 40 },
  { character: 'a', limit: 80 },
  { character: '😀', limit: 20 },
  { character: '👍🏽', limit: 10 },
  { character: '❤️', limit: 20 },
])('counts $character at its weighted local boundary', ({ character, limit }) => {
  expect(validateDanmakuMessage(character.repeat(limit - 1))).toBeUndefined()
  expect(validateDanmakuMessage(character.repeat(limit))).toBeUndefined()
  expect(validateDanmakuMessage(character.repeat(limit + 1))).toBe(danmakuMessageLengthError)
})

it.each([
  `${'a'.repeat(74)}中😀`,
  `${'e\u0301'.repeat(26)}ab`,
  `${'👨‍👩‍👧‍👦'.repeat(3)}${'a'.repeat(14)}`,
])('applies the weighted limit to mixed and combining content %j', (boundary) => {
  expect(validateDanmakuMessage(boundary)).toBeUndefined()
  expect(validateDanmakuMessage(`${boundary}a`)).toBe(danmakuMessageLengthError)
})

it.each(['', '   ', '\t\n'])('rejects blank content %j', (message) => {
  expect(validateDanmakuMessage(message)).toBe('请输入弹幕内容')
})

it.each(['文\u0000字', '文\t字', '文\n字', '文\u007f字', '文\u0085字', '\ud800', '\udfff'])(
  'rejects control characters and unpaired surrogates in %j',
  (message) => {
    expect(validateDanmakuMessage(message)).toBe('弹幕内容含有无效字符，请移除后重试')
  },
)

it('cleans a task queue without changing the content or order of nonempty lines', () => {
  expect(cleanDanmakuMessages('  第一条  \r\n\r\n 第二条 \n  \n😀')).toEqual([
    '第一条',
    '第二条',
    '😀',
  ])
})
