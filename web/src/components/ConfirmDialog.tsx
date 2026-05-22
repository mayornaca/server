import { useEffect, useRef, useState } from "react"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"

// ConfirmDialog replaces every `window.confirm()` in the panel. The native
// confirm blocks the entire tab (no devtools, no other tabs interactions),
// loses focus on Esc/click-outside in a way that hangs the awaiting promise
// and — most operationally relevant — has been observed leaving the panel in
// limbo for users who clicked but the action never persisted to the cloud.
// cloud-gesvial.19.1+.
//
// Usage:
//
//   const confirm = useConfirm()
//   ...
//   if (await confirm({ title: "...", message: "...", destructive: true })) {
//      doTheThing()
//   }
//
// The component itself only needs to be mounted ONCE near the app root —
// the `<ConfirmHost />` below renders the dialog. The hook returns a
// callable function bound to the host's state.

type ConfirmOptions = {
  title: string
  message?: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
}

type Pending = {
  options: ConfirmOptions
  resolve: (value: boolean) => void
}

let openConfirm: ((options: ConfirmOptions) => Promise<boolean>) | null = null

export function useConfirm() {
  return (options: ConfirmOptions): Promise<boolean> => {
    if (!openConfirm) {
      // <ConfirmHost /> is not mounted yet (before first render or app-root
      // miswire) — fall back to native confirm so the action is not silently
      // swallowed. This should only happen during dev startup.
      // eslint-disable-next-line no-alert
      return Promise.resolve(window.confirm(`${options.title}\n${options.message ?? ""}`))
    }
    return openConfirm(options)
  }
}

export function ConfirmHost() {
  const [pending, setPending] = useState<Pending | null>(null)
  const resolveRef = useRef<((v: boolean) => void) | null>(null)

  useEffect(() => {
    openConfirm = (options) => {
      return new Promise<boolean>((resolve) => {
        resolveRef.current = resolve
        setPending({ options, resolve })
      })
    }
    return () => {
      openConfirm = null
    }
  }, [])

  function close(result: boolean) {
    const pendingNow = pending
    setPending(null)
    if (pendingNow) {
      pendingNow.resolve(result)
    }
  }

  if (!pending) return null

  const opts = pending.options
  return (
    <Dialog open onOpenChange={(open) => !open && close(false)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{opts.title}</DialogTitle>
        </DialogHeader>
        {opts.message && (
          <p className="text-sm text-muted-foreground">{opts.message}</p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => close(false)}>
            {opts.cancelLabel ?? "Cancelar"}
          </Button>
          <Button
            variant={opts.destructive ? "destructive" : "default"}
            onClick={() => close(true)}
            autoFocus
          >
            {opts.confirmLabel ?? "Confirmar"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
