import { useCallback, useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { Card } from "@/components/ui/card"
import { StatusBadge } from "@/components/StatusBadge"
import { CardSkeleton, TableRowSkeleton } from "@/components/Skeleton"
import { posts, tests } from "@/api/client"
import type { PostsSummary, TestResult, SosPost } from "@/api/types"
import { formatDate } from "@/lib/utils"
import { usePanelEvents } from "@/hooks/usePanelEvents"

export default function Dashboard() {
  const [summary, setSummary] = useState<PostsSummary | null>(null)
  const [recentTests, setRecentTests] = useState<TestResult[]>([])
  const [allPosts, setAllPosts] = useState<SosPost[]>([])
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const [s, t, p] = await Promise.all([
        posts.summary(),
        tests.list({ limit: "10" }),
        // cloud-gesvial.19.1: 500 cap covers any realistic deployment for
        // the foreseeable future without paging the dashboard. Pre-fix the
        // 100 hardcap meant tests on the 101st+ post displayed raw UUIDs.
        posts.list({ limit: "500" }),
      ])
      setSummary(s.data)
      setRecentTests(t.data)
      setAllPosts(p.data)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error cargando datos")
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // Auto-refresh on relevant panel events. cloud-gesvial.19+.
  usePanelEvents(["test.scheduled", "test.completed", "post.statusChanged"], () => {
    void load()
  })

  if (error) return <p className="text-destructive">{error}</p>
  // cloud-gesvial.19.2 C4: skeletons en lugar de "Cargando..." plano para que
  // el operador vea visualmente la estructura mientras llega la data.
  if (!summary) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">Resumen</h1>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[0, 1, 2, 3].map((i) => <CardSkeleton key={i} />)}
        </div>
        <div>
          <h2 className="text-lg font-semibold mb-3">Tests recientes</h2>
          <div className="border rounded-md">
            <table className="w-full text-sm">
              <tbody>
                {[0, 1, 2, 3, 4].map((i) => <TableRowSkeleton key={i} cols={4} />)}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    )
  }

  const postMap = new Map(allPosts.map((p) => [p.id, p]))

  const cards = [
    { label: "Total Postes", value: summary.total, color: "text-foreground" },
    { label: "OK", value: summary.byStatus["OK"] || 0, color: "text-green-600" },
    { label: "FAIL", value: summary.byStatus["FAIL"] || 0, color: "text-red-600" },
    { label: "UNKNOWN", value: summary.byStatus["UNKNOWN"] || 0, color: "text-yellow-600" },
  ]

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Resumen</h1>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {cards.map((card) => (
          <Card key={card.label} className="p-4">
            <p className="text-sm text-muted-foreground">{card.label}</p>
            <p className={`text-3xl font-bold ${card.color}`}>{card.value}</p>
          </Card>
        ))}
      </div>

      {(summary.byStatus["TESTING"] || 0) > 0 && (
        <Card className="p-4 border-yellow-500/50 bg-yellow-50 dark:bg-yellow-950/20">
          <p className="text-sm font-medium">
            {summary.byStatus["TESTING"]} poste(s) en estado TESTING
          </p>
        </Card>
      )}

      <div>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold">Tests recientes</h2>
          <Link to="/tests" className="text-sm text-primary hover:underline">
            Ver todos
          </Link>
        </div>
        {recentTests.length === 0 ? (
          <p className="text-muted-foreground">Sin tests recientes</p>
        ) : (
          <div className="border rounded-md">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="text-left p-3">Fecha</th>
                  <th className="text-left p-3">Poste</th>
                  <th className="text-left p-3">Tipo</th>
                  <th className="text-left p-3">Estado</th>
                </tr>
              </thead>
              <tbody>
                {recentTests.map((t) => {
                  const post = postMap.get(t.postId)
                  return (
                    <tr key={t.id} className="border-b last:border-0">
                      <td className="p-3 text-muted-foreground text-xs">
                        {formatDate(t.CreatedAt)}
                      </td>
                      <td className="p-3">
                        {post ? (
                          <Link to={`/posts/${post.id}`} className="hover:underline">
                            {post.name}
                          </Link>
                        ) : (
                          <span className="font-mono text-xs">{t.postId.slice(0, 12)}</span>
                        )}
                      </td>
                      <td className="p-3">{t.testType}</td>
                      <td className="p-3">
                        <StatusBadge
                          status={t.status}
                          failureKind={t.failureKind}
                          deliveryConfirmed={t.deliveryConfirmed}
                        />
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
