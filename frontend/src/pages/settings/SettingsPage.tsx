import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, KeyRound, Plus, Trash2 } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { tokensApi } from '../../api/tokens'

const EXPIRY_OPTIONS = [
  { label: '30 days', days: 30 },
  { label: '90 days', days: 90 },
  { label: '1 year', days: 365 },
]

// The MCP server is reached through the gateway at /mcp — no local binary.
// Locally we use plain HTTP on :80 (avoids the self-signed-cert friction of the
// :443 route); for a deployed remote host we use HTTPS. Either way we target the
// gateway host, dropping any port (e.g. the :3000 Vite dev port, which is NOT
// the gateway and only speaks plain HTTP).
function mcpUrl() {
  const host = window.location.hostname
  const isLocal = host === 'localhost' || host === '127.0.0.1'
  return `${isLocal ? 'http' : 'https'}://${host}/mcp`
}

// One-liner for Claude Code — the caller's PAT rides along as a request header.
function claudeCodeCommand(token: string) {
  return `claude mcp add --transport http teamboard ${mcpUrl()} --header "Authorization: Bearer ${token}"`
}

// Ready-to-paste claude_desktop_config.json (HTTP transport) for the new token.
function claudeDesktopConfig(token: string) {
  return JSON.stringify({
    mcpServers: {
      teamboard: {
        type: 'http',
        url: mcpUrl(),
        headers: { Authorization: `Bearer ${token}` },
      },
    },
  }, null, 2)
}

function CreateTokenModal({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState('')
  const [expiresInDays, setExpiresInDays] = useState(90)
  const [error, setError] = useState('')
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [copied, setCopied] = useState<string | null>(null)
  const qc = useQueryClient()

  const create = useMutation({
    mutationFn: () => tokensApi.create({ name, expires_in_days: expiresInDays }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['tokens'] })
      setCreatedToken(res.data.token)
    },
    onError: (e: Error) => setError(e.message),
  })

  async function copy(what: string, text: string) {
    await navigator.clipboard.writeText(text)
    setCopied(what)
    setTimeout(() => setCopied(null), 2000)
  }

  function CopyButton({ what, text }: { what: string; text: string }) {
    return (
      <button type="button" onClick={() => copy(what, text)}
        className="btn-ghost flex-shrink-0 flex items-center gap-1.5">
        {copied === what ? <Check size={14} /> : <Copy size={14} />}
        {copied === what ? 'Copied' : 'Copy'}
      </button>
    )
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-bg-0/80 backdrop-blur-sm animate-fade-in">
      <div className="card w-full max-w-lg p-6 animate-scale-in max-h-[90vh] overflow-y-auto">
        {createdToken ? (
          <>
            <h2 className="text-base font-semibold text-text-0 mb-2">Token created</h2>
            <p className="text-sm text-text-2 mb-4">Copy this token now — it won't be shown again.</p>

            <label className="label block mb-1.5">Token</label>
            <div className="flex items-center gap-2 mb-5">
              <code className="input-base flex-1 text-xs break-all select-all">{createdToken}</code>
              <CopyButton what="token" text={createdToken} />
            </div>

            <label className="label block mb-1.5">Claude Code (one command)</label>
            <div className="flex items-start gap-2 mb-5">
              <pre className="input-base flex-1 text-xs overflow-x-auto whitespace-pre-wrap break-all select-all">{claudeCodeCommand(createdToken)}</pre>
              <CopyButton what="cli" text={claudeCodeCommand(createdToken)} />
            </div>

            <label className="label block mb-1.5">Claude Desktop config</label>
            <div className="flex items-start gap-2 mb-1">
              <pre className="input-base flex-1 text-xs overflow-x-auto whitespace-pre select-all">{claudeDesktopConfig(createdToken)}</pre>
              <CopyButton what="config" text={claudeDesktopConfig(createdToken)} />
            </div>
            <p className="text-xs text-text-3 mb-5">
              {mcpUrl().startsWith('https')
                ? <>The gateway uses a self-signed certificate — Claude Code needs <code className="mono">NODE_TLS_REJECT_UNAUTHORIZED=0</code> in <code className="mono">~/.claude/settings.json</code>. See <code className="mono">mcp-server/README.md</code>.</>
                : <>Run <code className="mono">make up-build</code> so the <code className="mono">mcp-server</code> container is up. See <code className="mono">mcp-server/README.md</code>.</>}
            </p>

            <div className="flex justify-end">
              <button onClick={onClose} className="btn-primary">Done</button>
            </div>
          </>
        ) : (
          <form onSubmit={(e) => { e.preventDefault(); setError(''); create.mutate() }} className="space-y-4">
            <h2 className="text-base font-semibold text-text-0 mb-1">New personal access token</h2>
            {error && (
              <div className="bg-danger/10 border border-danger/30 text-danger text-xs px-3 py-2 rounded">{error}</div>
            )}
            <div>
              <label className="label block mb-1.5">Name</label>
              <input value={name} onChange={(e) => setName(e.target.value)}
                className="input-base w-full" placeholder="MCP server" required autoFocus />
            </div>
            <div>
              <label className="label block mb-2">Expires in</label>
              <div className="grid grid-cols-3 gap-2">
                {EXPIRY_OPTIONS.map((opt) => (
                  <button key={opt.days} type="button" onClick={() => setExpiresInDays(opt.days)}
                    className={`py-1.5 rounded border text-xs font-medium transition-colors ${
                      expiresInDays === opt.days
                        ? 'bg-accent/10 border-accent text-accent'
                        : 'bg-bg-3 border-border-2 text-text-2 hover:border-border-3'
                    }`}>
                    {opt.label}
                  </button>
                ))}
              </div>
            </div>
            <div className="flex gap-2 justify-end pt-2">
              <button type="button" onClick={onClose} className="btn-ghost">Cancel</button>
              <button type="submit" disabled={create.isPending || !name.trim()} className="btn-primary">
                {create.isPending ? 'Creating…' : 'Create token'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}

export default function SettingsPage() {
  const [showCreateModal, setShowCreateModal] = useState(false)
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['tokens'],
    queryFn: () => tokensApi.list(),
  })

  const revoke = useMutation({
    mutationFn: (id: string) => tokensApi.revoke(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tokens'] }),
  })

  const tokens = data?.data ?? []

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <div className="mb-8">
        <h1 className="text-lg font-semibold text-text-0">Settings</h1>
      </div>

      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-sm font-semibold text-text-0">Personal access tokens</h2>
          <p className="text-xs text-text-2 mt-0.5">
            Let external tools (like an MCP server) read your projects, boards and tasks on your behalf.
          </p>
        </div>
        <button onClick={() => setShowCreateModal(true)}
          className="btn-primary flex items-center gap-2 flex-shrink-0">
          <Plus size={15} />
          New token
        </button>
      </div>

      {isLoading && (
        <div className="space-y-2">
          {[...Array(2)].map((_, i) => <div key={i} className="card h-16 animate-pulse bg-bg-2" />)}
        </div>
      )}

      {!isLoading && tokens.length === 0 && (
        <div className="card flex flex-col items-center justify-center py-16 text-center">
          <div className="w-12 h-12 bg-bg-2 rounded border border-border-1 flex items-center justify-center mb-3">
            <KeyRound size={20} className="text-text-3" />
          </div>
          <h3 className="text-sm font-medium text-text-1 mb-1">No tokens yet</h3>
          <p className="text-xs text-text-3">Create one to connect an external tool.</p>
        </div>
      )}

      {!isLoading && tokens.length > 0 && (
        <div className="card divide-y divide-border-1">
          {tokens.map((token) => {
            const isExpired = new Date(token.expires_at) < new Date()
            const isRevoked = !!token.revoked_at
            return (
              <div key={token.id} className="flex items-center justify-between px-4 py-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-text-0 truncate">{token.name}</span>
                    <code className="mono text-xs text-text-3">{token.token_prefix}…</code>
                    {isRevoked && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-danger/10 text-danger">Revoked</span>
                    )}
                    {!isRevoked && isExpired && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-bg-3 text-text-3">Expired</span>
                    )}
                  </div>
                  <p className="text-xs text-text-3 mt-0.5">
                    Created {formatDistanceToNow(new Date(token.created_at), { addSuffix: true })}
                    {' · '}
                    {isExpired ? 'Expired' : 'Expires'} {formatDistanceToNow(new Date(token.expires_at), { addSuffix: true })}
                    {token.last_used_at && ` · Last used ${formatDistanceToNow(new Date(token.last_used_at), { addSuffix: true })}`}
                  </p>
                </div>
                {!isRevoked && (
                  <button
                    onClick={() => {
                      if (window.confirm(`Revoke "${token.name}"? Any tool using it will stop working within a few seconds.`)) {
                        revoke.mutate(token.id)
                      }
                    }}
                    className="flex-shrink-0 p-1.5 text-text-3 hover:text-danger hover:bg-danger/5 rounded transition-colors"
                    title="Revoke token">
                    <Trash2 size={14} />
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}

      {showCreateModal && <CreateTokenModal onClose={() => setShowCreateModal(false)} />}
    </div>
  )
}
