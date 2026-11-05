import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchOpencodeGoQuota } from '@/lib/api'
import type { UsageIdentity } from '@/lib/types'
import { formatQuotaResetDuration, formatQuotaResetLabel } from './AuthFileCredentialsSection'
import styles from './OpencodeGoQuotaPanel.module.scss'

// 窗口展示顺序固定：5 小时 → 每周 → 每月，对应上游 usage 的三个滚动窗口。
// 依据 https://opencode.ai/v2/docs/console/go 「Usage limits」：
// 月额度 100%，5 小时窗口占 20%，每周占 50%。
const WINDOW_ORDER = ['rolling', 'weekly', 'monthly'] as const
const WINDOW_LABEL_KEY: Record<string, string> = {
  rolling: 'usage_stats.opencode_go_window_five_hour',
  weekly: 'usage_stats.opencode_go_window_weekly',
  monthly: 'usage_stats.opencode_go_window_monthly',
}

interface WindowRow {
  key: string
  label: string
  usedPercent: number
  resetsAt: string
  unavailable: boolean
}

/**
 * isOpencodeGoIdentity 判断某个凭证是否为 OpenCode Go 订阅渠道。
 *
 * opencode_go 在 CPA 里是 openai-compatibility 渠道，Keeper 侧 Type 为 openai、
 * Provider/展示名为 opencode_go，因此按 provider 名匹配（大小写与分隔符归一化）。
 */
export function isOpencodeGoIdentity(identity: Pick<UsageIdentity, 'provider' | 'type' | 'name' | 'alias'>): boolean {
  const candidates = [identity.provider, identity.type, identity.name, identity.alias]
  return candidates.some((value) => {
    const normalized = (value ?? '').trim().toLowerCase().replace(/[-_\s]+/g, '_')
    return normalized === 'opencode_go'
  })
}

/**
 * OpenCode Go 订阅额度面板：显示在 AI 供应商详情页的概览里。
 *
 * 该订阅是 CPA 的 API key 渠道，额度不走 CPA 通用额度接口，所以由 Keeper
 * 直连 OpenCode Go 的 usage 端点，展示 5 小时 / 每周 / 每月三个窗口的
 * 剩余额度与重置时间（与 antigravity、zcode 的额度条同款）。
 */
export function OpencodeGoQuotaPanel({ timeZone }: { timeZone?: string }) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<WindowRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  // 404 表示 keeper 未配置 OPENCODE_GO_API_KEY，此时整个板块隐藏。
  const [hidden, setHidden] = useState(false)

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    try {
      const response = await fetchOpencodeGoQuota(signal)
      const usage = response.usage?.usage ?? {}
      const next: WindowRow[] = []
      for (const window of WINDOW_ORDER) {
        const entry = usage[window]
        if (!entry) continue
        next.push({
          key: window,
          label: t(WINDOW_LABEL_KEY[window]),
          usedPercent: typeof entry.percent === 'number' ? entry.percent : 0,
          resetsAt: entry.resetsAt ?? '',
          // 上游 status 非 ok 表示该窗口当前不可用。
          unavailable: entry.status ? entry.status.toLowerCase() !== 'ok' : false,
        })
      }
      if (next.length === 0) {
        setHidden(true)
      } else {
        setRows(next)
      }
    } catch (err) {
      if (signal?.aborted) return
      // 404 → 未配置，隐藏板块；其它错误就地提示。
      if (err instanceof Error && /404/.test(err.message)) {
        setHidden(true)
      } else {
        setError(err instanceof Error ? err.message : String(err))
      }
    } finally {
      if (!signal?.aborted) setLoading(false)
    }
  }, [t])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  if (hidden) {
    return null
  }

  return (
    <div className={styles.quotaPanel}>
      {loading && <div className={styles.message}>{t('usage_stats.credentials_quota_loading')}</div>}
      {!loading && error && <div className={styles.error}>{error}</div>}
      {!loading && !error && rows.map((row) => {
        // 上游给的是已用百分比；进度条表达剩余额度，与其它 provider 一致。
        const remaining = Math.max(0, Math.min(100, 100 - row.usedPercent))
        const tone = row.unavailable || remaining < 20 ? 'danger' : remaining < 50 ? 'warning' : 'ok'
        const resetLabel = row.resetsAt ? formatQuotaResetLabel(row.resetsAt, timeZone) : ''
        const resetDuration = row.resetsAt ? formatQuotaResetDuration(row.resetsAt) : ''
        return (
          <div key={row.key} className={styles.barBlock}>
            <div className={styles.barHeader}>
              <span>{row.label}</span>
              <span className={styles.barValue}>
                {resetDuration && <span className={styles.resetDuration}>{resetDuration}</span>}
                <strong>{Math.round(remaining)}%</strong>
              </span>
            </div>
            <div className={styles.track}>
              <span className={`${styles.fill} ${styles[tone]}`} style={{ width: `${remaining}%` }} />
            </div>
            {resetLabel && <div className={styles.barFooter}><span>{resetLabel}</span></div>}
          </div>
        )
      })}
      <div className={styles.panelFooter}>
        <button type="button" className={styles.refreshButton} onClick={() => void load()} disabled={loading}>
          {t('usage_stats.opencode_go_refresh')}
        </button>
      </div>
    </div>
  )
}
