// Shared presentation for native buttons and links used as actions.
export const buttonBase =
  'ui-button relative inline-flex items-center justify-center gap-2 rounded-[10px] border px-4 text-sm font-medium transition-colors duration-200 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring disabled:cursor-not-allowed disabled:opacity-50 aria-disabled:cursor-not-allowed aria-disabled:opacity-50'

export const buttonSizes = {
  small: 'ui-button--small h-[30px] px-2.5 text-xs',
  default: 'ui-button--default h-9',
  large: 'h-11',
} as const

export const buttonVariants = {
  primary:
    'border-transparent bg-accent-solid text-on-accent hover:bg-accent-text active:brightness-90',
  secondary:
    'border-border-strong bg-surface-solid text-text-primary hover:bg-surface-solid-hover active:bg-surface-muted',
  ghost:
    'border-transparent bg-transparent text-text-secondary hover:bg-surface-muted hover:text-text-primary active:bg-surface-solid-hover',
  danger:
    'border-transparent bg-danger-solid text-on-danger hover:brightness-110 active:brightness-90',
} as const

export type ButtonVariant = keyof typeof buttonVariants
export type ButtonSize = keyof typeof buttonSizes

export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'large') {
  return `${buttonBase} ${buttonSizes[size]} ${buttonVariants[variant]}`
}

export const primaryButton = buttonClass('primary')
export const secondaryButton = buttonClass('secondary')
