"use client"

import * as React from "react"
import { Laptop, Moon, Sun } from "lucide-react"
import { useTheme } from "next-themes"

import { cn } from "@/lib/utils"

const themes = [
  { value: "light", label: "Light", icon: Sun },
  { value: "system", label: "System", icon: Laptop },
  { value: "dark", label: "Dark", icon: Moon },
] as const

export function ThemeSwitcher({ className }: { className?: string }) {
  const { theme, setTheme } = useTheme()
  const mounted = React.useSyncExternalStore(
    () => () => undefined,
    () => true,
    () => false
  )

  return (
    <div
      aria-label="Color theme"
      className={cn(
        "grid h-8 grid-cols-3 rounded-[var(--radius-control)] bg-input p-0.5 shadow-[var(--shadow-inset-field)]",
        className
      )}
      role="group"
    >
      {themes.map(({ value, label, icon: Icon }) => {
        const active = mounted && theme === value

        return (
          <button
            key={value}
            type="button"
            aria-label={`${label} theme`}
            aria-pressed={active}
            className={cn(
              "flex min-w-0 items-center justify-center gap-1 rounded-[6px] px-1.5 text-[10.5px] font-medium text-muted-foreground transition-[background-color,color,box-shadow] duration-150 ease-[var(--ease-out-strong)] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              active && "bg-card text-foreground shadow-[var(--shadow-button)]"
            )}
            onClick={() => setTheme(value)}
          >
            <Icon aria-hidden="true" className="size-3" />
            <span className="hidden 2xl:inline">{label}</span>
          </button>
        )
      })}
    </div>
  )
}
