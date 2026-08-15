'use client'

import { useCallback, useMemo, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

const GRID = 8

/** Lit dots forming the S — row/col 0-indexed */
const LIT_DOTS = [
  { row: 0, col: 2, accent: false },
  { row: 0, col: 3, accent: false },
  { row: 0, col: 4, accent: false },
  { row: 0, col: 5, accent: false },
  { row: 0, col: 6, accent: false },
  { row: 0, col: 7, accent: true },
  { row: 1, col: 1, accent: false },
  { row: 2, col: 1, accent: false },
  { row: 3, col: 2, accent: false },
  { row: 3, col: 3, accent: false },
  { row: 3, col: 4, accent: false },
  { row: 3, col: 5, accent: false },
  { row: 4, col: 5, accent: false },
  { row: 5, col: 5, accent: false },
  { row: 6, col: 1, accent: false },
  { row: 6, col: 2, accent: false },
  { row: 6, col: 3, accent: false },
  { row: 6, col: 4, accent: false },
  { row: 6, col: 5, accent: false },
  { row: 7, col: 0, accent: true },
  { row: 7, col: 1, accent: false },
  { row: 7, col: 2, accent: false },
  { row: 7, col: 3, accent: false },
  { row: 7, col: 4, accent: false },
  { row: 7, col: 5, accent: false },
  { row: 7, col: 6, accent: false },
] as const

type DotPhase = 'idle' | 'scatter' | 'return'

function scatterOffset(row: number, col: number, magnitude: number) {
  const seed = row * GRID + col
  const angle = ((seed * 137.508) % 360) * (Math.PI / 180)
  const dist = magnitude * (0.65 + (seed % 5) * 0.12)
  return {
    x: Math.cos(angle) * dist,
    y: Math.sin(angle) * dist,
  }
}

function dotKey(row: number, col: number) {
  return `${row}-${col}`
}

interface LogoMarkProps {
  size?: number
  className?: string
  interactive?: boolean
}

export function LogoMark({ size = 32, className, interactive = true }: LogoMarkProps) {
  const [phase, setPhase] = useState<DotPhase>('idle')
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const litMap = useMemo(() => {
    const map = new Map<string, { accent: boolean }>()
    for (const dot of LIT_DOTS) {
      map.set(dotKey(dot.row, dot.col), { accent: dot.accent })
    }
    return map
  }, [])

  const offsets = useMemo(() => {
    const magnitude = size * 0.55
    const map = new Map<string, { x: number; y: number }>()
    for (const dot of LIT_DOTS) {
      map.set(dotKey(dot.row, dot.col), scatterOffset(dot.row, dot.col, magnitude))
    }
    return map
  }, [size])

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const runScatterCycle = useCallback(() => {
    clearTimer()
    setPhase('scatter')
    timerRef.current = setTimeout(() => {
      setPhase('return')
      timerRef.current = setTimeout(() => {
        setPhase('idle')
      }, 1000)
    }, 400)
  }, [clearTimer])

  const handleEnter = () => {
    if (!interactive) return
    if (phase === 'idle') runScatterCycle()
  }

  const handleLeave = () => {
    if (!interactive) return
    clearTimer()
    setPhase('return')
    timerRef.current = setTimeout(() => setPhase('idle'), 1000)
  }

  const gap = size * 0.08
  const dotSize = (size - gap * (GRID - 1)) / GRID

  return (
    <div
      className={cn(
        'relative shrink-0 rounded-[var(--radius-control)] bg-card shadow-[var(--shadow-button)]',
        interactive && 'cursor-pointer',
        className,
      )}
      style={{ width: size, height: size }}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      aria-hidden
    >
      <div
        className="grid h-full w-full"
        style={{
          gridTemplateColumns: `repeat(${GRID}, ${dotSize}px)`,
          gridTemplateRows: `repeat(${GRID}, ${dotSize}px)`,
          gap,
        }}
      >
        {Array.from({ length: GRID * GRID }, (_, i) => {
          const row = Math.floor(i / GRID)
          const col = i % GRID
          const key = dotKey(row, col)
          const lit = litMap.get(key)
          const offset = offsets.get(key)
          const scattered = phase === 'scatter' && lit
          const returning = phase === 'return' && lit

          let transform = 'translate(0, 0)'
          let transition = 'transform 0ms'

          if (lit && offset) {
            if (scattered) {
              transform = `translate(${offset.x}px, ${offset.y}px)`
              transition = 'transform 400ms cubic-bezier(0.22, 1, 0.36, 1)'
            } else if (returning || phase === 'idle') {
              transform = 'translate(0, 0)'
              transition = returning
                ? 'transform 1000ms cubic-bezier(0.34, 1.2, 0.64, 1)'
                : 'transform 0ms'
            }
          }

          return (
            <span
              key={key}
              className={cn(
                'rounded-full',
                lit
                  ? lit.accent
                    ? 'bg-[var(--design-accent)] shadow-[0_0_4px_color-mix(in_srgb,var(--design-accent)_35%,transparent)]'
                    : 'bg-foreground shadow-[0_0_3px_color-mix(in_srgb,var(--foreground)_20%,transparent)]'
                  : 'bg-border/70',
              )}
              style={{
                width: dotSize,
                height: dotSize,
                transform,
                transition,
                willChange: lit ? 'transform' : undefined,
              }}
            />
          )
        })}
      </div>
    </div>
  )
}
