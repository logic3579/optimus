import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN.json'
import enUS from './en-US.json'
import zhCNAntd from 'ant-design-vue/es/locale/zh_CN'
import enUSAntd from 'ant-design-vue/es/locale/en_US'

export type SupportedLocale = 'zh-CN' | 'en-US'

export const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'en-US',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS
  }
})

export function antdLocale(loc: SupportedLocale) {
  return loc === 'zh-CN' ? zhCNAntd : enUSAntd
}
