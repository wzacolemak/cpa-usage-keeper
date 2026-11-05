import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchDeepSeekBalance } from '@/lib/api'
import type { UsageIdentity } from '@/lib/types'
import styles from './DeepSeekBalancePanel.module.scss'

interface BalanceRow {
  currency: string
  total: number
  granted: number
  toppedUp: number
}

/**
 * isDeepSeekIdentity 判断某个凭证是否为 DeepSeek 官方渠道。
 *
 * DeepSeek 在 CPA 里是 openai-compatibility 渠道（provider 名 deepseek），
 * 按 provider/type/name/alias 归一化后匹配。
 */
export function isDeepSeekIdentity(identity: Pick<UsageIdentity, 'provider' | 'type' | 'name' | 'alias'>): boolean {
  const candidates = [identity.provider, identity.type, identity.name, identity.alias]
  return candidates.some((value) => {
    const normalized = (value ?? '').trim().toLowerCase().replace(/[-_\s]+/g, '_')
    // 排除 deepseek 系模型名（如 deepseek-v4.1-flash），只认渠道名。
    return normalized === 'deepseek' || normalized === 'openai_compatible_deepseek'
  })
}

function toNumber(value: string | undefined): number {
  const parsed = Number.parseFloat((value ?? '').trim())
  return Number.isFinite(parsed) ? parsed : 0
}

function formatAmount(value: number, currency: string): string {
  const symbol = currency.toUpperCase() === 'CNY' ? '¥' : currency.toUpperCase() === 'USD' ? '$' : ''
  return `${symbol}${value.toFixed(2)}`
}

/**
 * DeepSeek 余额面板：显示在 AI 供应商详情页的概览里。
 *
 * DeepSeek 官方只有 GET /user/balance（余额），没有用量/账单 API，
 * 因此这里只展示余额数值；token 与成本统计仍由 Keeper 从 usage 事件算出。
 * 余额是绝对值，没有「上限」可比，所以不画进度条。
 */
export function DeepSeekBalancePanel({ timeZone: _timeZone }: { timeZone?: string }) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<BalanceRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  // 404 表示 keeper 未配置 DEEPSEEK_API_KEY，此时整个板块隐藏。
  const [hidden, setHidden] = useState(false)

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    try {
      const response = await fetchDeepSeekBalance(signal)
      const next: BalanceRow[] = (response.balances ?? []).map((item) => ({
        currency: (item.currency ?? '').toUpperCase(),
        total: toNumber(item.total_balance),
        granted: toNumber(item.granted_balance),
        toppedUp: toNumber(item.topped_up_balance),
      }))
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
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  if (hidden) {
    return null
  }

  return (
    <div className={styles.panel}>
      {loading && <div className={styles.message}>{t('usage_stats.credentials_quota_loading')}</div>}
      {!loading && error && <div className={styles.error}>{error}</div>}
      {!loading && !error && rows.map((row) => (
        <div key={row.currency} className={styles.balanceBlock}>
          <div className={styles.balanceRow}>
            <span className={styles.balanceCurrency}>{row.currency}</span>
            <strong className={styles.balanceAmount}>{formatAmount(row.total, row.currency)}</strong>
            <span className={styles.balanceLabel}>{t('usage_stats.deepseek_balance_total')}</span>
          </div>
          <div className={styles.balanceDetail}>
            <span>
              {t('usage_stats.deepseek_balance_granted')}: {formatAmount(row.granted, row.currency)}
            </span>
            <span>
              {t('usage_stats.deepseek_balance_topped_up')}: {formatAmount(row.toppedUp, row.currency)}
            </span>
          </div>
        </div>
      ))}
      <div className={styles.panelFooter}>
        <button type="button" className={styles.refreshButton} onClick={() => void load()} disabled={loading}>
          {t('usage_stats.opencode_go_refresh')}
        </button>
      </div>
    </div>
  )
}
