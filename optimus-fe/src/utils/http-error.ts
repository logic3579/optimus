export class BizError extends Error {
  readonly code: number
  readonly messageKey?: string
  constructor(code: number, message: string, messageKey?: string) {
    super(message)
    this.name = 'BizError'
    this.code = code
    this.messageKey = messageKey
  }
}

export function isBizError(e: unknown): e is BizError {
  return e instanceof BizError
}

export function parseEnvelopeError(body: unknown): BizError | null {
  if (!body || typeof body !== 'object') return null
  const env = body as { code?: unknown; message?: unknown; message_key?: unknown }
  if (typeof env.code !== 'number' || env.code === 0) return null
  const msg = typeof env.message === 'string' ? env.message : ''
  const key = typeof env.message_key === 'string' ? env.message_key : undefined
  return new BizError(env.code, msg, key)
}
