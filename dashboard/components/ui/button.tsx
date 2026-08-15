import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-[var(--radius-control)] text-[12.5px] font-medium tracking-[-0.01em] ring-offset-background transition-[background-color,color,box-shadow,transform,filter] duration-150 ease-[var(--ease-out-strong)] active:scale-[0.97] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-40 disabled:active:scale-100 [&_svg]:pointer-events-none [&_svg]:size-3.5 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground shadow-[var(--shadow-solid-button)] hover:brightness-[1.08]",
        destructive:
          "bg-destructive text-destructive-foreground shadow-[var(--shadow-solid-button)] hover:brightness-[1.08]",
        outline:
          "bg-card text-foreground shadow-[var(--shadow-button)] hover:bg-accent hover:text-accent-foreground",
        secondary:
          "bg-secondary text-secondary-foreground shadow-[var(--shadow-hairline)] hover:bg-accent",
        ghost:
          "text-muted-foreground shadow-none hover:bg-accent hover:text-foreground",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-8 px-3",
        sm: "h-7 px-2.5 text-xs",
        lg: "h-9 px-4 text-[13px]",
        xl: "h-10 px-5 text-sm",
        icon: "size-8 px-0",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button"
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = "Button"

export { Button, buttonVariants }
