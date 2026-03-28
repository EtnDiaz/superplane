import { Cloud, Box, Shield, ExternalLink, Copy, CheckCircle, AlertCircle } from "lucide-react";
import { usePageTitle } from "@/hooks/usePageTitle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useQuery } from "@tanstack/react-query";
import { useCanvases } from "@/hooks/useCanvasData";
import { useState } from "react";
import type { LucideIcon } from "lucide-react";

interface SandboxesProps {
  organizationId: string;
}

type ProviderKey = "docker" | "gvisor" | "cloudflare";

interface ProviderStatus {
  available: boolean;
  reason?: string;
}

interface SandboxStatusResponse {
  providers: {
    docker: ProviderStatus;
    gvisor: ProviderStatus;
    cloudflare: ProviderStatus;
  };
}

interface ProviderConfig {
  key: ProviderKey;
  icon: LucideIcon;
  name: string;
  description: string;
  languages: string[];
}

const PROVIDERS: ProviderConfig[] = [
  {
    key: "gvisor",
    icon: Shield,
    name: "gVisor",
    description:
      "Runs code via Docker with the gVisor (runsc) runtime. Provides kernel-level isolation by intercepting system calls in user space.",
    languages: ["Python", "Node.js", "Go", "Ruby", "Java", "Bash"],
  },
  {
    key: "docker",
    icon: Box,
    name: "Docker",
    description: "Runs code in standard Docker containers. Easiest to set up locally or on a self-hosted instance.",
    languages: ["Python", "Node.js", "Go", "Ruby", "Java", "Bash", "PHP", "Rust"],
  },
  {
    key: "cloudflare",
    icon: Cloud,
    name: "Cloudflare Workers",
    description:
      "Routes code execution to a deployed Cloudflare Bridge Worker. No infrastructure to manage on your end.",
    languages: ["JavaScript", "TypeScript", "Python (via WASM)", "Rust (via WASM)"],
  },
];

function StatusBadge({ available, reason }: { available: boolean; reason?: string }) {
  if (available) {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-green-700 dark:text-green-400">
        <span className="w-1.5 h-1.5 rounded-full bg-green-500 shrink-0" />
        Available
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-red-600 dark:text-red-400">
      <span className="w-1.5 h-1.5 rounded-full bg-red-500 shrink-0" />
      Unavailable{reason ? ` — ${reason}` : ""}
    </span>
  );
}

function ProviderCardSkeleton() {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-300 dark:border-gray-700 p-6 animate-pulse">
      <div className="flex items-start gap-3">
        <div className="w-9 h-9 rounded-md bg-gray-200 dark:bg-gray-700 shrink-0" />
        <div className="flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <div className="h-4 w-24 bg-gray-200 dark:bg-gray-700 rounded" />
            <div className="h-4 w-16 bg-gray-200 dark:bg-gray-700 rounded" />
          </div>
          <div className="h-3 w-3/4 bg-gray-200 dark:bg-gray-700 rounded" />
        </div>
      </div>
      <div className="mt-4 flex gap-1.5">
        {[1, 2, 3].map((i) => (
          <div key={i} className="h-5 w-14 bg-gray-200 dark:bg-gray-700 rounded" />
        ))}
      </div>
    </div>
  );
}

interface ProviderCardProps {
  provider: ProviderConfig;
  status: ProviderStatus | undefined;
  isLoading: boolean;
}

function ProviderCard({ provider, status, isLoading }: ProviderCardProps) {
  const Icon = provider.icon;

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-300 dark:border-gray-700 overflow-hidden">
      <div className="px-6 pt-6 pb-4">
        <div className="flex items-start gap-3">
          <div className="flex-shrink-0 w-9 h-9 rounded-md bg-gray-100 dark:bg-gray-700 flex items-center justify-center">
            <Icon size={18} className="text-gray-700 dark:text-gray-300" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h3 className="text-sm font-semibold text-gray-900 dark:text-white">{provider.name}</h3>
              {isLoading ? (
                <div className="h-4 w-20 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
              ) : status ? (
                <StatusBadge available={status.available} reason={status.reason} />
              ) : null}
            </div>
            <p className="mt-1.5 text-sm text-gray-600 dark:text-gray-400">{provider.description}</p>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-1.5">
          {provider.languages.map((lang) => (
            <span
              key={lang}
              className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300"
            >
              {lang}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

interface CloudflareDeployResponse {
  workerUrl: string;
}

interface CloudflareDeploySectionProps {
  organizationId: string;
}

function CloudflareDeploySection({ organizationId }: CloudflareDeploySectionProps) {
  const [accountId, setAccountId] = useState("");
  const [apiToken, setApiToken] = useState("");
  const [authToken, setAuthToken] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [deployedUrl, setDeployedUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const handleDeploy = async () => {
    setIsLoading(true);
    setError(null);
    setDeployedUrl(null);

    try {
      const response = await fetch("/api/v1/sandbox/cloudflare/deploy", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "x-org-id": organizationId,
        },
        body: JSON.stringify({ accountId, apiToken, authToken }),
      });

      const data = (await response.json()) as CloudflareDeployResponse;

      if (!response.ok) {
        const errData = data as unknown as { error?: string; message?: string };
        setError(errData.error ?? errData.message ?? `Deploy failed: ${response.statusText}`);
        return;
      }

      setDeployedUrl(data.workerUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : "An unexpected error occurred");
    } finally {
      setIsLoading(false);
    }
  };

  const handleCopy = async () => {
    if (!deployedUrl) return;
    await navigator.clipboard.writeText(deployedUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-300 dark:border-gray-700 overflow-hidden">
      <div className="px-6 pt-6 pb-6">
        <div className="flex items-start gap-3 mb-5">
          <div className="flex-shrink-0 w-9 h-9 rounded-md bg-orange-50 dark:bg-orange-900/20 flex items-center justify-center">
            <Cloud size={18} className="text-orange-600 dark:text-orange-400" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-gray-900 dark:text-white">
              Cloudflare Dynamic Workers — Auto Deploy
            </h3>
            <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
              Deploy a Cloudflare Bridge Worker to your account. Once deployed, use the URL in Canvas Settings →
              Sandbox Runtime → Cloudflare.
            </p>
          </div>
        </div>

        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Account ID</label>
            <Input
              placeholder="Cloudflare Account ID"
              value={accountId}
              onChange={(e) => setAccountId(e.target.value)}
              disabled={isLoading}
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">API Token</label>
            <Input
              type="password"
              placeholder="CF API token with Workers Scripts:Edit permission"
              value={apiToken}
              onChange={(e) => setApiToken(e.target.value)}
              disabled={isLoading}
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Auth Token</label>
            <Input
              type="password"
              placeholder="Secret token for securing the Bridge Worker"
              value={authToken}
              onChange={(e) => setAuthToken(e.target.value)}
              disabled={isLoading}
            />
          </div>

          <Button
            onClick={handleDeploy}
            disabled={isLoading || !accountId || !apiToken || !authToken}
            className="mt-1"
          >
            {isLoading ? "Deploying…" : "Deploy Bridge Worker"}
          </Button>
        </div>

        {deployedUrl && (
          <div className="mt-4 rounded-md bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 p-4">
            <div className="flex items-start gap-2">
              <CheckCircle size={16} className="text-green-600 dark:text-green-400 mt-0.5 shrink-0" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-green-800 dark:text-green-300">Bridge Worker deployed!</p>
                <div className="mt-2 flex items-center gap-2">
                  <code className="text-xs text-green-700 dark:text-green-400 bg-green-100 dark:bg-green-900/40 px-2 py-1 rounded font-mono truncate flex-1">
                    {deployedUrl}
                  </code>
                  <button
                    onClick={handleCopy}
                    className="shrink-0 p-1.5 rounded hover:bg-green-100 dark:hover:bg-green-900/40 text-green-700 dark:text-green-400 transition-colors"
                    title="Copy URL"
                  >
                    {copied ? <CheckCircle size={14} /> : <Copy size={14} />}
                  </button>
                </div>
                <p className="mt-2 text-xs text-green-700 dark:text-green-400">
                  Copy this URL and use it in Canvas Settings → Sandbox Runtime → Cloudflare
                </p>
              </div>
            </div>
          </div>
        )}

        {error && (
          <div className="mt-4 rounded-md bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 p-4">
            <div className="flex items-start gap-2">
              <AlertCircle size={16} className="text-red-600 dark:text-red-400 mt-0.5 shrink-0" />
              <p className="text-sm text-red-700 dark:text-red-400">{error}</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

interface CanvasRowProps {
  name: string;
  provider: string;
  href: string;
}

function CanvasRow({ name, provider, href }: CanvasRowProps) {
  return (
    <a
      href={href}
      className="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors group"
    >
      <Badge variant="secondary" className="text-xs font-mono shrink-0">
        {provider}
      </Badge>
      <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 truncate">{name}</span>
      <ExternalLink
        size={14}
        className="text-gray-400 dark:text-gray-500 opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
      />
    </a>
  );
}

export function Sandboxes({ organizationId }: SandboxesProps) {
  usePageTitle(["Sandboxes"]);

  const { data: statusData, isLoading: statusLoading } = useQuery<SandboxStatusResponse>({
    queryKey: ["sandbox-status", organizationId],
    queryFn: async () => {
      const response = await fetch("/api/v1/sandbox/status", {
        headers: {
          "x-org-id": organizationId,
        },
      });
      if (!response.ok) {
        throw new Error(`Failed to fetch sandbox status: ${response.statusText}`);
      }
      return response.json() as Promise<SandboxStatusResponse>;
    },
    enabled: !!organizationId,
  });

  const { data: canvases = [], isLoading: canvasesLoading } = useCanvases(organizationId);

  const sandboxCanvases = canvases.filter(
    (canvas) => canvas.metadata?.sandboxProvider && canvas.metadata.sandboxProvider !== "",
  );

  return (
    <div className="space-y-4 pt-6">
      <CloudflareDeploySection organizationId={organizationId} />

      {statusLoading
        ? PROVIDERS.map((p) => <ProviderCardSkeleton key={p.key} />)
        : PROVIDERS.map((p) => (
            <ProviderCard key={p.key} provider={p} status={statusData?.providers[p.key]} isLoading={false} />
          ))}

      <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-300 dark:border-gray-700 overflow-hidden">
        <div className="px-6 pt-6 pb-4">
          <p className="text-sm font-medium text-gray-600 dark:text-gray-300">
            Sandbox-enabled canvases
            {!canvasesLoading && (
              <span className="ml-1 text-gray-400 dark:text-gray-500">({sandboxCanvases.length})</span>
            )}
          </p>
        </div>

        {canvasesLoading ? (
          <div className="px-6 pb-6 space-y-2">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-10 bg-gray-100 dark:bg-gray-700 rounded animate-pulse" />
            ))}
          </div>
        ) : sandboxCanvases.length === 0 ? (
          <div className="px-6 pb-6">
            <p className="text-sm text-gray-500 dark:text-gray-400">
              No canvases are using sandbox isolation yet. Enable it in Canvas Settings.
            </p>
          </div>
        ) : (
          <div className="border-t border-gray-200 dark:border-gray-700 divide-y divide-gray-100 dark:divide-gray-700/50">
            {sandboxCanvases.map((canvas) => {
              const id = canvas.metadata?.id ?? "";
              const name = canvas.metadata?.name ?? id;
              const provider = canvas.metadata?.sandboxProvider ?? "";
              return <CanvasRow key={id} name={name} provider={provider} href={`/${organizationId}/canvases/${id}`} />;
            })}
          </div>
        )}
      </div>
    </div>
  );
}
