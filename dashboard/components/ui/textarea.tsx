import * as React from "react"

import { cn } from "@/lib/utils"

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<"textarea">
>(({ className, ...props }, ref) => {
  return (
    <textarea
      className={cn(
        "flex min-h-20 w-full resize-y rounded-[var(--radius-control)] border-0 bg-input px-3 py-2.5 text-sm text-foreground shadow-[var(--shadow-inset-field)] ring-offset-background transition-[background-color,box-shadow] duration-150 ease-[var(--ease-out-strong)] placeholder:text-muted-foreground focus-visible:outline-none focus-visible:shadow-[var(--shadow-focus-field)] disabled:cursor-not-allowed disabled:opacity-40",
        className
      )}
      ref={ref}
      {...props}
    />
  )
})
Textarea.displayName = "Textarea"

export { Textarea }
