import { ref } from 'vue'

export type Locale = 'en' | 'zh-CN'

const messages = {
  en: {
    dashboard: 'Dashboard', online: 'Online', offline: 'Unavailable', refreshed: 'Updated',
    usage: 'Token usage', usageNote: 'Input and output tokens processed by this bridge',
    totalTokens: 'Total tokens', remaining: 'Codex remaining', requests: 'Requests', latency: 'Average latency',
    input: 'Input', output: 'Output', quota: 'Subscription limits', balance: 'Credit balance',
    primary: 'Primary window', secondary: 'Secondary window', resets: 'Resets', unknown: 'Not reported',
    unlimited: 'Unlimited', noExtraCredits: 'No extra credits',
    managedInChatGPT: 'Managed in ChatGPT', officialUsage: 'Official usage',
    quotaUnavailable: 'This connection did not report subscription limits. View the authoritative usage and app limits in ChatGPT.',
    viewUsage: 'View usage in ChatGPT',
    models: 'Configured models', tier: 'Tier', model: 'Model ID', service: 'Service', uptime: 'Uptime',
    errorRate: 'Error rate', dataWindow: 'Data retention', localOnly: 'Metrics remain on this device.',
    empty: 'No requests in this period', retry: 'Retry', hours24: '24 hours',
  },
  'zh-CN': {
    dashboard: '仪表盘', online: '运行中', offline: '暂不可用', refreshed: '更新时间',
    usage: 'Token 用量', usageNote: '经由此桥接服务处理的输入与输出 Token',
    totalTokens: 'Token 总量', remaining: 'Codex 剩余额度', requests: '请求数', latency: '平均延迟',
    input: '输入', output: '输出', quota: '订阅额度', balance: '积分余额',
    primary: '主要窗口', secondary: '次要窗口', resets: '重置时间', unknown: '暂未报告',
    unlimited: '无限额', noExtraCredits: '无额外积分',
    managedInChatGPT: '由 ChatGPT 管理', officialUsage: '官方用量',
    quotaUnavailable: '当前连接未返回订阅额度。请在 ChatGPT 中查看权威用量和应用限额。',
    viewUsage: '在 ChatGPT 中查看用量',
    models: '已配置模型', tier: '等级', model: '模型 ID', service: '服务状态', uptime: '运行时间',
    errorRate: '错误率', dataWindow: '数据保留', localOnly: '指标仅保存在本机。',
    empty: '该时段暂无请求', retry: '重试', hours24: '24 小时',
  },
} as const

const saved = localStorage.getItem('codex-bridge-locale')
export const locale = ref<Locale>(saved === 'en' || saved === 'zh-CN' ? saved : navigator.language.startsWith('zh') ? 'zh-CN' : 'en')

export function setLocale(value: Locale) {
  locale.value = value
  localStorage.setItem('codex-bridge-locale', value)
  document.documentElement.lang = value
}

export function t(key: keyof typeof messages.en): string {
  return messages[locale.value][key]
}
