export default function ResourceState({ loading, error, retry }: { loading?: boolean; error?: Error | null; retry?: () => void }) {
  if (error) return <div role="alert" className="mb-4 rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{error.message}{retry && <button onClick={retry} className="btn btn-secondary ml-3">重试</button>}</div>
  return loading ? <p role="status" className="app-muted mb-4">正在加载…</p> : null
}
