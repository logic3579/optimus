export function newIdempotencyKey() {
  const cryptoProvider = globalThis.crypto
  if (cryptoProvider?.randomUUID) return cryptoProvider.randomUUID()
  const bytes = new Uint8Array(16)
  if (cryptoProvider?.getRandomValues) cryptoProvider.getRandomValues(bytes)
  else for (let index = 0; index < bytes.length; index++) bytes[index] = Math.floor(Math.random() * 256)
  bytes[6] = ((bytes[6] ?? 0) & 15) | 64
  bytes[8] = ((bytes[8] ?? 0) & 63) | 128
  return [bytes.slice(0, 4), bytes.slice(4, 6), bytes.slice(6, 8), bytes.slice(8, 10), bytes.slice(10)]
    .map(part => Array.from(part, byte => byte.toString(16).padStart(2, '0')).join('')).join('-')
}
