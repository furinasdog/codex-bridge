export type RangeKey = '1h' | '5h' | '12h' | '24h'

export interface UsageBucket {
  start: string
  input_tokens: number
  output_tokens: number
  requests: number
}

export interface UsageRange {
  hours: number
  totals: {
    input_tokens: number
    output_tokens: number
    requests: number
    errors: number
  }
  buckets: UsageBucket[]
}

export interface LimitWindow {
  used_percent: number | null
  remaining_percent: number | null
  reset_at: string | null
  window_minutes: number | null
}

export interface DashboardResponse {
  service: {
    generated_at: string
    started_at: string
    uptime_seconds: number
    average_latency_ms: number
    error_rate: number
    ranges: Record<RangeKey, UsageRange>
    quota: {
      plan_type?: string
      balance: number | null
      has_credits: boolean | null
      unlimited: boolean | null
      primary: LimitWindow
      secondary: LimitWindow
      last_updated: string | null
    }
  }
  models: Array<{
    id: string
    display_name: string
    tier: string
  }>
}
