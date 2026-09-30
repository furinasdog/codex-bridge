<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import UsageChart from './components/UsageChart.vue'
import { locale, setLocale, t } from './i18n'
import type { DashboardResponse, LimitWindow, RangeKey } from './types'

const data = ref<DashboardResponse | null>(null)
const loading = ref(true)
const error = ref(false)
const selectedRange = ref<RangeKey>('5h')
const ranges: RangeKey[] = ['1h', '5h', '12h', '24h']
let timer: number | undefined

const currentRange = computed(() => data.value?.service.ranges[selectedRange.value])
const totalTokens = computed(() => (currentRange.value?.totals.input_tokens ?? 0) + (currentRange.value?.totals.output_tokens ?? 0))
const quotaReported = computed(() => {
  const quota = data.value?.service.quota
  return Boolean(quota && (
    quota.plan_type ||
    quota.balance != null ||
    quota.has_credits != null ||
    quota.unlimited != null ||
    quota.primary.remaining_percent != null ||
    quota.secondary.remaining_percent != null
  ))
})

async function load() {
  try {
    const response = await fetch('/api/dashboard', { headers: { Accept: 'application/json' } })
    if (!response.ok) throw new Error(String(response.status))
    data.value = await response.json()
    error.value = false
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

function number(value: number, compact = true) {
  return new Intl.NumberFormat(locale.value, compact ? { notation: 'compact', maximumFractionDigits: 1 } : { maximumFractionDigits: 1 }).format(value)
}

function duration(seconds: number) {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return `${hours}h ${minutes}m`
}

function date(value: string | null | undefined) {
  if (!value) return t('unknown')
  return new Intl.DateTimeFormat(locale.value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

function remaining(window: LimitWindow) {
  return window.remaining_percent == null ? t('unknown') : `${number(window.remaining_percent, false)}%`
}

function creditBalance() {
  const quota = data.value?.service.quota
  if (!quota) return t('unknown')
  if (quota.unlimited) return t('unlimited')
  if (quota.has_credits === false) return t('noExtraCredits')
  if (quota.balance != null) return number(quota.balance, false)
  return t('unknown')
}

onMounted(() => {
  document.documentElement.lang = locale.value
  load()
  timer = window.setInterval(load, 10_000)
})
onBeforeUnmount(() => {
  if (timer !== undefined) window.clearInterval(timer)
})
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <div class="brand"><span class="mark">CB</span><span>Codex Bridge</span></div>
      <div class="top-actions">
        <span class="status" :class="{ down: error }"><i></i>{{ error ? t('offline') : t('online') }}</span>
        <div class="language" aria-label="Language">
          <button :class="{ active: locale === 'en' }" @click="setLocale('en')">EN</button>
          <button :class="{ active: locale === 'zh-CN' }" @click="setLocale('zh-CN')">中文</button>
        </div>
      </div>
    </header>

    <section class="intro">
      <div><p class="eyebrow">{{ t('dashboard') }}</p><h1>{{ t('usage') }}</h1><p>{{ t('usageNote') }}</p></div>
      <span v-if="data" class="updated">{{ t('refreshed') }} · {{ date(data.service.generated_at) }}</span>
    </section>

    <div v-if="loading" class="loading"><span></span><span></span><span></span></div>
    <div v-else-if="error && !data" class="error-state"><p>{{ t('offline') }}</p><button @click="load">{{ t('retry') }}</button></div>

    <template v-else-if="data && currentRange">
      <section class="stats-grid">
        <article><span>{{ t('totalTokens') }}</span><strong>{{ number(totalTokens) }}</strong><small>{{ selectedRange }}</small></article>
        <article><span>{{ t('remaining') }}</span><strong>{{ quotaReported ? remaining(data.service.quota.primary) : t('managedInChatGPT') }}</strong><small>{{ data.service.quota.plan_type || t('officialUsage') }}</small></article>
        <article><span>{{ t('requests') }}</span><strong>{{ number(currentRange.totals.requests, false) }}</strong><small>{{ selectedRange }}</small></article>
        <article><span>{{ t('latency') }}</span><strong>{{ number(data.service.average_latency_ms, false) }} ms</strong><small>{{ t('service') }}</small></article>
      </section>

      <section class="panel usage-panel">
        <div class="panel-heading">
          <div><h2>{{ t('usage') }}</h2><p>{{ number(currentRange.totals.input_tokens) }} {{ t('input').toLowerCase() }} · {{ number(currentRange.totals.output_tokens) }} {{ t('output').toLowerCase() }}</p></div>
          <div class="range-tabs">
            <button v-for="range in ranges" :key="range" :class="{ active: selectedRange === range }" @click="selectedRange = range">{{ range }}</button>
          </div>
        </div>
        <UsageChart :buckets="currentRange.buckets" :locale="locale" :input-label="t('input')" :output-label="t('output')" :empty-label="t('empty')" />
      </section>

      <section class="two-column">
        <article class="panel quota-panel">
          <div class="panel-heading"><div><h2>{{ t('quota') }}</h2><p>{{ data.service.quota.plan_type || t('unknown') }}</p></div><div class="quota-balance"><small>{{ t('balance') }}</small><strong>{{ creditBalance() }}</strong></div></div>
          <div v-if="!quotaReported" class="quota-unreported">
            <p>{{ t('quotaUnavailable') }}</p>
            <a href="https://chatgpt.com/settings/usage" target="_blank" rel="noopener noreferrer">{{ t('viewUsage') }} <span aria-hidden="true">↗</span></a>
          </div>
          <div v-else class="quota-row" v-for="item in [{ label: t('primary'), value: data.service.quota.primary }, { label: t('secondary'), value: data.service.quota.secondary }]" :key="item.label">
            <div><span>{{ item.label }}</span><strong>{{ remaining(item.value) }}</strong></div>
            <div class="meter"><i :style="{ width: `${item.value.remaining_percent ?? 0}%` }"></i></div>
            <small>{{ t('resets') }} · {{ date(item.value.reset_at) }}</small>
          </div>
        </article>

        <article class="panel service-panel">
          <div class="panel-heading"><div><h2>{{ t('service') }}</h2><p>{{ t('localOnly') }}</p></div></div>
          <dl>
            <div><dt>{{ t('uptime') }}</dt><dd>{{ duration(data.service.uptime_seconds) }}</dd></div>
            <div><dt>{{ t('errorRate') }}</dt><dd>{{ number(data.service.error_rate, false) }}%</dd></div>
            <div><dt>{{ t('dataWindow') }}</dt><dd>{{ t('hours24') }}</dd></div>
          </dl>
        </article>
      </section>

      <section class="panel models-panel">
        <div class="panel-heading"><div><h2>{{ t('models') }}</h2><p>{{ data.models.length }}</p></div></div>
        <div class="model-table">
          <div class="model-row head"><span>{{ t('tier') }}</span><span>{{ t('model') }}</span></div>
          <div class="model-row" v-for="model in data.models" :key="model.id"><span>{{ model.display_name }}</span><code>{{ model.id }}</code></div>
        </div>
      </section>
    </template>
  </main>
</template>
