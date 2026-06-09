import { describe, it, expect } from 'vitest'
import { BizError, parseEnvelopeError, isBizError } from './http-error'

describe('BizError', () => {
  it('carries code, message, message_key', () => {
    const e = new BizError(40101, 'invalid creds', 'auth.invalid_credentials')
    expect(e.code).toBe(40101)
    expect(e.message).toBe('invalid creds')
    expect(e.messageKey).toBe('auth.invalid_credentials')
    expect(isBizError(e)).toBe(true)
  })

  it('isBizError discriminates against plain Error', () => {
    expect(isBizError(new Error('x'))).toBe(false)
  })
})

describe('parseEnvelopeError', () => {
  it('returns BizError from a non-zero envelope', () => {
    const e = parseEnvelopeError({ code: 50001, data: null, message: 'oops', message_key: 'k' })
    expect(e).toBeInstanceOf(BizError)
    expect(e?.code).toBe(50001)
  })

  it('returns null for code=0 envelopes', () => {
    expect(parseEnvelopeError({ code: 0, data: {}, message: '' })).toBeNull()
  })

  it('returns null for non-object inputs', () => {
    expect(parseEnvelopeError(null)).toBeNull()
    expect(parseEnvelopeError(undefined)).toBeNull()
    expect(parseEnvelopeError('whatever')).toBeNull()
  })
})
