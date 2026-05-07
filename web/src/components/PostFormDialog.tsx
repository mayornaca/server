import { useState, useEffect, type FormEvent } from "react"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useConfirm } from "@/components/ConfirmDialog"
import { posts as postsApi } from "@/api/client"
import type { SosPost } from "@/api/types"

interface Props {
  open: boolean
  post?: SosPost | null
  onClose: () => void
  onSaved: () => void
}

export function PostFormDialog({ open, post, onClose, onSaved }: Props) {
  const confirm = useConfirm()
  const [name, setName] = useState("")
  const [kmMarker, setKmMarker] = useState("")
  const [phoneNumber, setPhoneNumber] = useState("")
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (post) {
      setName(post.name)
      setKmMarker(post.kmMarker)
      setPhoneNumber(post.phoneNumber)
    } else {
      setName("")
      setKmMarker("")
      setPhoneNumber("")
    }
    setError(null)
  }, [post, open])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      if (post) {
        await postsApi.update(post.id, { name, kmMarker, phoneNumber })
      } else {
        await postsApi.create({ name, kmMarker, phoneNumber })
      }
      onSaved()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error al guardar")
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    if (!post) return
    const ok = await confirm({
      title: "Eliminar poste SOS",
      message: `Se eliminará "${post.name}" (KM ${post.kmMarker}). Esta acción no se puede deshacer.`,
      confirmLabel: "Eliminar",
      destructive: true,
    })
    if (!ok) return
    setSaving(true)
    try {
      await postsApi.delete(post.id)
      onSaved()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error al eliminar")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{post ? "Editar Poste SOS" : "Nuevo Poste SOS"}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="text-sm font-medium mb-1 block">Nombre</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="SOS-100"
              required
              maxLength={128}
            />
          </div>
          <div>
            <label className="text-sm font-medium mb-1 block">Marcador KM</label>
            <Input
              value={kmMarker}
              onChange={(e) => setKmMarker(e.target.value)}
              placeholder="42.5"
              required
              maxLength={32}
            />
          </div>
          <div>
            <label className="text-sm font-medium mb-1 block">Teléfono</label>
            <Input
              value={phoneNumber}
              onChange={(e) => setPhoneNumber(e.target.value)}
              placeholder="+56912345678"
              required
              maxLength={20}
              disabled={!!post}
            />
            {post && (
              <p className="text-xs text-muted-foreground mt-1">
                El teléfono no se puede modificar
              </p>
            )}
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter className="flex justify-between">
            {post && (
              <Button
                type="button"
                variant="destructive"
                onClick={handleDelete}
                disabled={saving}
              >
                Eliminar
              </Button>
            )}
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={onClose} disabled={saving}>
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? "Guardando..." : post ? "Guardar" : "Crear"}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
