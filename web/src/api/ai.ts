import request from './request'

export interface AICharacter {
  id: string
  name: string
  avatar: string
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

/** 获取 AI 角色列表 */
export function getAICharacters() {
  return request.get('/ai/characters') as Promise<{ list: AICharacter[] }>
}

/** 与 AI 角色流式对话（SSE） */
export async function chatWithAIStream(
  characterId: string,
  messages: ChatMessage[],
  onChunk: (content: string) => void
): Promise<void> {
  const token = localStorage.getItem('fake_bili_token') || ''
  const resp = await fetch('/api/v1/ai/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'x-access-token': token,
    },
    body: JSON.stringify({
      character_id: characterId,
      messages,
    }),
  })

  if (!resp.ok || !resp.body) {
    throw new Error('AI 服务请求失败')
  }

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })

    // SSE 事件以空行（\n\n）分隔
    let sep: number
    while ((sep = buffer.indexOf('\n\n')) !== -1) {
      const rawEvent = buffer.slice(0, sep)
      buffer = buffer.slice(sep + 2)

      const dataLine = rawEvent
        .split('\n')
        .find((l) => l.startsWith('data:'))
      if (!dataLine) continue

      const data = dataLine.slice(5).trim()
      if (data === '[DONE]') return

      try {
        const parsed = JSON.parse(data)
        if (parsed.error) {
          throw Object.assign(new Error(String(parsed.error)), { aiError: true })
        }
        if (parsed.content) onChunk(parsed.content as string)
      } catch (e) {
        // 解析分片失败（数据未完整）忽略；显式抛出的业务错误需上抛
        if (e instanceof Error && (e as { aiError?: boolean }).aiError) throw e
      }
    }
  }
}
