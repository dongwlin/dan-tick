// Keep this local limit aligned with internal/danmaku's offline validation.
export const danmakuMessageLimit = 80
export const danmakuMessageLengthError = '弹幕内容超过本应用的长度上限，请缩短后重试'
export const danmakuMessageLimitDescription =
  '本应用上限相当于 80 个英文字母或数字、40 个普通汉字或 20 个 😀 表情；混合内容按比例计数，组合表情可能占更多长度。'

export function validateDanmakuMessage(message: string): string | undefined {
  if (!message.trim()) return '请输入弹幕内容'
  // Unicode mode matches an unpaired surrogate but allows valid emoji pairs.
  if (/\p{Cc}|[\uD800-\uDFFF]/u.test(message)) return '弹幕内容含有无效字符，请移除后重试'
  let length = 0
  for (const character of message) {
    const codepoint = character.codePointAt(0) ?? 0
    // The official webpage counts printable ASCII as 1 and every other UTF-16 unit as 2.
    length += codepoint >= 0x20 && codepoint <= 0x7e ? 1 : codepoint <= 0xffff ? 2 : 4
    if (length > danmakuMessageLimit) return danmakuMessageLengthError
  }
}

export function cleanDanmakuMessages(messages: string): string[] {
  return messages
    .split(/\r?\n/)
    .map((message) => message.trim())
    .filter(Boolean)
}
