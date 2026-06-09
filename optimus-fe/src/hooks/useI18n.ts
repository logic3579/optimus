import { useI18n as baseUseI18n } from 'vue-i18n'
import type { SupportedLocale } from '@/locales'

export function useI18n() {
  return baseUseI18n<{ message: Record<string, unknown> }, SupportedLocale>({ useScope: 'global' })
}
