import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = join(process.cwd(), 'src/views/assets')

function vueFiles(dir: string, files: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (entry === '__tests__') continue
    if (statSync(path).isDirectory()) vueFiles(path, files)
    else if (entry.endsWith('.vue')) files.push(path)
  }
  return files
}

const expectedGates: Record<string, string[]> = {
  'cloud-accounts/Form.vue': [],
  'cloud-accounts/List.vue': [
    'assets:account:write',
    'assets:account:write',
    'assets:account:write',
    'assets:account:delete',
  ],
  'databases/List.vue': [],
  'instances/List.vue': [],
  'sync-runs/List.vue': [],
  'vpcs/Detail.vue': [],
  'vpcs/List.vue': [],
}

const expectedOperations: Record<string, Array<{ testID: string; permission: string }>> = {
  'cloud-accounts/List.vue': [
    { testID: 'data-testid="create-account"', permission: 'assets:account:write' },
    { testID: ':data-testid="`sync-${record.id}`"', permission: 'assets:account:write' },
    { testID: ':data-testid="`edit-${record.id}`"', permission: 'assets:account:write' },
    { testID: ':data-testid="`delete-${record.id}`"', permission: 'assets:account:delete' },
  ],
}

describe('assets v-permission audit', () => {
  const files = vueFiles(root)

  it('enumerates every assets page so new pages cannot bypass the audit', () => {
    expect(files.map(file => relative(root, file)).sort()).toEqual(Object.keys(expectedGates).sort())
  })

  for (const file of files) {
    const name = relative(root, file)
    it(`${name} declares exactly its audited mutation gates`, () => {
      const source = readFileSync(file, 'utf8')
      const declared = [...source.matchAll(/v-permission=["']'([^"']+)'["']/g)]
        .map(match => match[1])
        .sort()
      expect(declared).toEqual([...(expectedGates[name] ?? [])].sort())

      for (const operation of expectedOperations[name] ?? []) {
        const marker = source.indexOf(operation.testID)
        expect(marker, `${name} missing ${operation.testID}`).toBeGreaterThanOrEqual(0)
        const openingTag = source.slice(source.lastIndexOf('<', marker), source.indexOf('>', marker) + 1)
        expect(openingTag).toContain(`v-permission="'${operation.permission}'"`)
      }
    })
  }
})
