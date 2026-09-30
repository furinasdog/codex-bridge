<script setup lang="ts">
import { computed } from 'vue'
import type { UsageBucket } from '../types'

const props = defineProps<{ buckets: UsageBucket[]; locale: string; inputLabel: string; outputLabel: string; emptyLabel: string }>()

const width = 960
const height = 260
const padding = { top: 18, right: 54, bottom: 34, left: 54 }
const plotWidth = width - padding.left - padding.right
const plotHeight = height - padding.top - padding.bottom

const inputMaximum = computed(() => Math.max(1, ...props.buckets.map((item) => item.input_tokens)))
const outputMaximum = computed(() => Math.max(1, ...props.buckets.map((item) => item.output_tokens)))
const hasData = computed(() => props.buckets.some((item) => item.input_tokens + item.output_tokens > 0))

function point(index: number, value: number, maximum: number) {
  const denominator = Math.max(1, props.buckets.length - 1)
  return [padding.left + (index / denominator) * plotWidth, padding.top + plotHeight - (value / maximum) * plotHeight]
}

function line(selector: (bucket: UsageBucket) => number, maximum: number) {
  return props.buckets.map((bucket, index) => {
    const [x, y] = point(index, selector(bucket), maximum)
    return `${index === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
}

const inputLine = computed(() => line((bucket) => bucket.input_tokens, inputMaximum.value))
const outputLine = computed(() => line((bucket) => bucket.output_tokens, outputMaximum.value))
const labels = computed(() => {
  if (!props.buckets.length) return []
  const indexes = [0, Math.floor((props.buckets.length - 1) / 2), props.buckets.length - 1]
  return indexes.map((index) => ({
    index,
    text: new Intl.DateTimeFormat(props.locale, { hour: '2-digit', minute: '2-digit' }).format(new Date(props.buckets[index].start)),
    x: point(index, 0, 1)[0],
  }))
})
</script>

<template>
  <div class="chart-wrap">
    <div class="chart-legend" aria-hidden="true">
      <span><i class="legend-total"></i>{{ inputLabel }}</span>
      <span><i class="legend-output"></i>{{ outputLabel }}</span>
    </div>
    <svg class="usage-chart" :viewBox="`0 0 ${width} ${height}`" role="img" :aria-label="`${inputLabel} / ${outputLabel}`">
      <g class="grid">
        <line v-for="step in 4" :key="step" :x1="padding.left" :x2="width - padding.right" :y1="padding.top + (plotHeight / 4) * step" :y2="padding.top + (plotHeight / 4) * step" />
      </g>
      <text class="axis-label axis-input" :x="padding.left - 10" :y="padding.top + 4" text-anchor="end">{{ inputMaximum.toLocaleString(locale) }}</text>
      <text class="axis-label" :x="padding.left - 10" :y="padding.top + plotHeight + 4" text-anchor="end">0</text>
      <text class="axis-label axis-output" :x="width - padding.right + 10" :y="padding.top + 4" text-anchor="start">{{ outputMaximum.toLocaleString(locale) }}</text>
      <text class="axis-label axis-output" :x="width - padding.right + 10" :y="padding.top + plotHeight + 4" text-anchor="start">0</text>
      <path v-if="hasData" class="line-total" :d="inputLine" />
      <path v-if="hasData" class="line-output" :d="outputLine" />
      <text v-if="!hasData" class="empty-label" :x="padding.left + plotWidth / 2" :y="padding.top + plotHeight / 2" text-anchor="middle">{{ emptyLabel }}</text>
      <text v-for="label in labels" :key="label.index" class="axis-label" :x="label.x" :y="height - 8" :text-anchor="label.index === 0 ? 'start' : label.index === buckets.length - 1 ? 'end' : 'middle'">{{ label.text }}</text>
    </svg>
  </div>
</template>
