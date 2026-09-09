import { useEffect, useRef } from 'react'
import { AlertTriangle, X } from 'lucide-react'

interface ConfirmDialogProps {
  open: boolean
  title: string
  description?: string
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export default function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = '确认',
  cancelLabel = '取消',
  destructive = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    cancelRef.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => event.key === 'Escape' && onCancel()
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onCancel])

  if (!open) return null

  return (
    <div className="modal-backdrop fixed inset-0 z-[70] flex items-end justify-center p-3 sm:items-center" onMouseDown={onCancel}>
      <div role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby={description ? 'confirm-description' : undefined} onMouseDown={event => event.stopPropagation()} className="modal-panel app-surface w-full max-w-md rounded-2xl border p-5 shadow-2xl">
        <div className="flex items-start gap-3">
          <span className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-xl ${destructive ? 'bg-red-50 text-red-600 dark:bg-red-950/50 dark:text-red-300' : 'bg-amber-50 text-amber-600 dark:bg-amber-950/50 dark:text-amber-300'}`}><AlertTriangle size={21} /></span>
          <div className="min-w-0 flex-1"><h2 id="confirm-title" className="text-lg font-semibold">{title}</h2>{description && <p id="confirm-description" className="app-muted mt-1.5 text-sm leading-6">{description}</p>}</div>
          <button onClick={onCancel} className="icon-button -mr-2 -mt-2" aria-label="关闭确认窗口"><X size={19} /></button>
        </div>
        <div className="mt-6 grid grid-cols-2 gap-2 sm:flex sm:justify-end">
          <button ref={cancelRef} onClick={onCancel} className="btn btn-secondary sm:min-w-24">{cancelLabel}</button>
          <button onClick={onConfirm} className={`btn sm:min-w-24 ${destructive ? 'btn-danger' : 'btn-primary'}`}>{confirmLabel}</button>
        </div>
      </div>
    </div>
  )
}
