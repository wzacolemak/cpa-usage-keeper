import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { fetchPricingTimeWindows, updatePricingTimeWindows } from '@/lib/api'
import type { PricingTimeWindowSpec, PricingTimeWindows } from '@/lib/types'
import styles from './PriceTimeWindowsPanel.module.scss'

const WEEKDAY_ORDER = [1, 2, 3, 4, 5, 6, 0]

interface WindowDraft {
  id: string
  name: string
  weekdays: number[]
  ranges: string
}

let nextWindowID = 0

const createWindowDraft = (spec?: PricingTimeWindowSpec): WindowDraft => ({
  id: `time-window-${++nextWindowID}`,
  name: spec?.name ?? '',
  weekdays: spec?.weekdays ?? [],
  ranges: (spec?.ranges ?? []).join(', '),
})

/**
 * PriceTimeWindowsPanel 编辑分时段计价的时段定义。
 *
 * 这里只定义「几点到几点叫什么」，具体倍率仍由各模型的价格规则
 * （time_window=<时段名>）决定，因此同一时段可以是不同模型的 2× 或 1.5×。
 */
export function PriceTimeWindowsPanel({ onNotice }: { onNotice?: (message: string) => void }) {
  const { t } = useTranslation()
  const [timezone, setTimezone] = useState('Asia/Shanghai')
  const [drafts, setDrafts] = useState<WindowDraft[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [expanded, setExpanded] = useState(false)

  const applyResponse = useCallback((response: PricingTimeWindows) => {
    setTimezone(response.timezone || 'Asia/Shanghai')
    setDrafts((response.windows ?? []).map(createWindowDraft))
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      try {
        const response = await fetchPricingTimeWindows(controller.signal)
        applyResponse(response)
      } catch (err) {
        if (!controller.signal.aborted) {
          setError(err instanceof Error ? err.message : String(err))
        }
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    })()
    return () => controller.abort()
  }, [applyResponse])

  const updateDraft = useCallback((id: string, patch: Partial<WindowDraft>) => {
    setDrafts((current) => current.map((draft) => (draft.id === id ? { ...draft, ...patch } : draft)))
  }, [])

  const toggleWeekday = useCallback((id: string, weekday: number) => {
    setDrafts((current) => current.map((draft) => {
      if (draft.id !== id) return draft
      const has = draft.weekdays.includes(weekday)
      const weekdays = has
        ? draft.weekdays.filter((value) => value !== weekday)
        : [...draft.weekdays, weekday].sort((left, right) => left - right)
      return { ...draft, weekdays }
    }))
  }, [])

  const weekdayLabels = useMemo(() => ({
    1: t('usage_stats.pricing_time_window_monday'),
    2: t('usage_stats.pricing_time_window_tuesday'),
    3: t('usage_stats.pricing_time_window_wednesday'),
    4: t('usage_stats.pricing_time_window_thursday'),
    5: t('usage_stats.pricing_time_window_friday'),
    6: t('usage_stats.pricing_time_window_saturday'),
    0: t('usage_stats.pricing_time_window_sunday'),
  } as Record<number, string>), [t])

  const handleSave = useCallback(async () => {
    setSaving(true)
    setError('')
    try {
      const payload: PricingTimeWindows = {
        timezone: timezone.trim(),
        windows: drafts
          .map((draft) => ({
            name: draft.name.trim(),
            weekdays: draft.weekdays,
            ranges: draft.ranges
              .split(',')
              .map((range) => range.trim())
              .filter(Boolean),
          }))
          .filter((spec) => spec.name !== '' && spec.ranges.length > 0),
      }
      const response = await updatePricingTimeWindows(payload)
      applyResponse(response)
      onNotice?.(t('usage_stats.pricing_time_windows_saved'))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }, [applyResponse, drafts, onNotice, t, timezone])

  const summary = drafts.length === 0
    ? t('usage_stats.pricing_time_windows_disabled')
    : drafts.map((draft) => draft.name).filter(Boolean).join(', ')

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <button
          type="button"
          className={styles.toggle}
          onClick={() => setExpanded((value) => !value)}
          aria-expanded={expanded}
        >
          {expanded ? '▾' : '▸'} {t('usage_stats.pricing_time_windows_title')}
        </button>
        <span className={styles.summary}>{summary}</span>
      </div>

      {expanded && (
        <div className={styles.body}>
          <p className={styles.hint}>{t('usage_stats.pricing_time_windows_hint')}</p>
          {loading ? (
            <div className={styles.hint}>{t('common.loading')}</div>
          ) : (
            <>
              <div className={styles.field}>
                <label>{t('usage_stats.pricing_time_windows_timezone')}</label>
                <input
                  value={timezone}
                  onChange={(event) => setTimezone(event.target.value)}
                  placeholder="Asia/Shanghai"
                  className={styles.input}
                  disabled={saving}
                />
              </div>

              {drafts.map((draft) => (
                <div key={draft.id} className={styles.windowRow}>
                  <input
                    value={draft.name}
                    onChange={(event) => updateDraft(draft.id, { name: event.target.value })}
                    placeholder={t('usage_stats.pricing_time_windows_name_placeholder')}
                    className={styles.input}
                    disabled={saving}
                  />
                  <input
                    value={draft.ranges}
                    onChange={(event) => updateDraft(draft.id, { ranges: event.target.value })}
                    placeholder="09:00-12:00, 14:00-18:00"
                    className={styles.input}
                    disabled={saving}
                  />
                  <div className={styles.weekdays}>
                    {WEEKDAY_ORDER.map((weekday) => (
                      <button
                        key={weekday}
                        type="button"
                        className={draft.weekdays.includes(weekday) ? styles.weekdayActive : styles.weekday}
                        onClick={() => toggleWeekday(draft.id, weekday)}
                        disabled={saving}
                        title={weekdayLabels[weekday]}
                      >
                        {weekdayLabels[weekday]}
                      </button>
                    ))}
                  </div>
                  <button
                    type="button"
                    className={styles.remove}
                    onClick={() => setDrafts((current) => current.filter((item) => item.id !== draft.id))}
                    disabled={saving}
                  >
                    ×
                  </button>
                </div>
              ))}

              <div className={styles.actions}>
                <button
                  type="button"
                  className={styles.secondary}
                  onClick={() => setDrafts((current) => [...current, createWindowDraft()])}
                  disabled={saving}
                >
                  {t('usage_stats.pricing_time_windows_add')}
                </button>
                <button
                  type="button"
                  className={styles.primary}
                  onClick={() => void handleSave()}
                  disabled={saving}
                >
                  {saving ? t('common.saving') : t('common.save')}
                </button>
              </div>
              {error && <div className={styles.error}>{error}</div>}
            </>
          )}
        </div>
      )}
    </div>
  )
}
