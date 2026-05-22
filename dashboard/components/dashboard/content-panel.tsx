import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

interface ContentPanelProps {
  title: string
  children: React.ReactNode
  className?: string
  contentClassName?: string
}

export function ContentPanel({ title, children, className, contentClassName }: ContentPanelProps) {
  return (
    <Card className={cn('overflow-hidden shadow-sm', className)}>
      <CardHeader className="border-b border-border bg-muted/30 px-5 py-3.5">
        <CardTitle className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className={cn('p-5', contentClassName)}>{children}</CardContent>
    </Card>
  )
}
