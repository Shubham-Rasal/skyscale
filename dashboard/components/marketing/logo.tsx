import Link from 'next/link'
import { LogoMark } from '@/components/marketing/logo-mark'
import { cn } from '@/lib/utils'

export function SkyscaleLogo({
  className,
  href = '/',
  subtitle,
  interactive = true,
  showLabel = true,
  size = 32,
}: {
  className?: string
  href?: string
  subtitle?: string
  interactive?: boolean
  showLabel?: boolean
  size?: number
}) {
  return (
    <Link href={href} className={cn('group flex items-center gap-2.5', className)}>
      <LogoMark size={size} interactive={interactive} />
      {showLabel && (
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold tracking-tight text-foreground">Skyscale</p>
          {subtitle && (
            <p className="truncate text-[11px] text-muted-foreground">{subtitle}</p>
          )}
        </div>
      )}
    </Link>
  )
}
