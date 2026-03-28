import { Cloud, Box, Shield } from "lucide-react";
import { usePageTitle } from "@/hooks/usePageTitle";
import { Badge } from "@/components/ui/badge";
import type { LucideIcon } from "lucide-react";

interface SandboxesProps {
  organizationId: string;
}

interface ProviderCardProps {
  icon: LucideIcon;
  name: string;
  badge: string;
  badgeVariant: "default" | "secondary" | "outline";
  description: string;
  languages: string[];
  configTitle: string;
  configContent: React.ReactNode;
}

function ProviderCard({
  icon: Icon,
  name,
  badge,
  badgeVariant,
  description,
  languages,
  configTitle,
  configContent,
}: ProviderCardProps) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-300 dark:border-gray-700 overflow-hidden">
      <div className="px-6 pt-6 pb-4">
        <div className="flex items-start gap-3">
          <div className="flex-shrink-0 w-9 h-9 rounded-md bg-gray-100 dark:bg-gray-700 flex items-center justify-center">
            <Icon size={18} className="text-gray-700 dark:text-gray-300" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h3 className="text-sm font-semibold text-gray-900 dark:text-white">{name}</h3>
              <Badge variant={badgeVariant} className="text-xs">
                {badge}
              </Badge>
            </div>
            <p className="mt-1.5 text-sm text-gray-600 dark:text-gray-400">{description}</p>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-1.5">
          {languages.map((lang) => (
            <span
              key={lang}
              className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300"
            >
              {lang}
            </span>
          ))}
        </div>
      </div>

      <div className="px-6 pb-6">
        <div className="rounded-md bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 p-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400 mb-2">
            {configTitle}
          </p>
          {configContent}
        </div>
      </div>
    </div>
  );
}

export function Sandboxes({ organizationId: _organizationId }: SandboxesProps) {
  usePageTitle(["Sandboxes"]);

  return (
    <div className="space-y-4 pt-6">
      <ProviderCard
        icon={Shield}
        name="gVisor"
        badge="Most Secure"
        badgeVariant="default"
        description="Runs code via Docker with the gVisor (runsc) runtime. Provides kernel-level isolation by intercepting system calls in user space. Requires gVisor installed on the SuperPlane host."
        languages={["Python", "Node.js", "Go", "Ruby", "Java", "Bash"]}
        configTitle="How to configure"
        configContent={
          <div className="space-y-2">
            <p className="text-xs text-gray-600 dark:text-gray-400">
              Set <code className="font-mono bg-gray-200 dark:bg-gray-700 px-1 py-0.5 rounded text-xs">SANDBOX_PROVIDER=gvisor</code> in your SuperPlane environment, then run containers with the gVisor runtime:
            </p>
            <pre className="text-xs font-mono bg-gray-900 text-gray-100 rounded p-3 overflow-x-auto">
              {`docker run --runtime=runsc \\
  --rm -i your-image \\
  <command>`}
            </pre>
            <p className="text-xs text-gray-500 dark:text-gray-500">
              Install gVisor:{" "}
              <a
                href="https://gvisor.dev/docs/user_guide/install/"
                target="_blank"
                rel="noopener noreferrer"
                className="text-sky-600 dark:text-sky-400 hover:underline"
              >
                gvisor.dev/docs/user_guide/install
              </a>
            </p>
          </div>
        }
      />

      <ProviderCard
        icon={Box}
        name="Docker"
        badge="Easy Setup"
        badgeVariant="secondary"
        description="Runs code in standard Docker containers. Easiest to set up locally or on a self-hosted instance. Requires Docker to be installed and running on the SuperPlane host."
        languages={["Python", "Node.js", "Go", "Ruby", "Java", "Bash", "PHP", "Rust"]}
        configTitle="How to configure"
        configContent={
          <div className="space-y-2">
            <p className="text-xs text-gray-600 dark:text-gray-400">
              Set <code className="font-mono bg-gray-200 dark:bg-gray-700 px-1 py-0.5 rounded text-xs">SANDBOX_PROVIDER=docker</code> in your SuperPlane environment. Docker must be accessible from the SuperPlane process:
            </p>
            <pre className="text-xs font-mono bg-gray-900 text-gray-100 rounded p-3 overflow-x-auto">
              {`# Ensure Docker socket is accessible
SANDBOX_PROVIDER=docker
SANDBOX_DOCKER_IMAGE=superplane/sandbox:latest`}
            </pre>
          </div>
        }
      />

      <ProviderCard
        icon={Cloud}
        name="Cloudflare Workers"
        badge="Zero Infra"
        badgeVariant="outline"
        description="Routes code execution to a deployed Cloudflare Bridge Worker. No infrastructure to manage on your end. Requires a Cloudflare account and a deployed Bridge Worker."
        languages={["JavaScript", "TypeScript", "Python (via WASM)", "Rust (via WASM)"]}
        configTitle="How to configure"
        configContent={
          <div className="space-y-2">
            <p className="text-xs text-gray-600 dark:text-gray-400">
              Deploy the SuperPlane Bridge Worker to your Cloudflare account, then configure the endpoint and API token:
            </p>
            <pre className="text-xs font-mono bg-gray-900 text-gray-100 rounded p-3 overflow-x-auto">
              {`SANDBOX_PROVIDER=cloudflare
SANDBOX_CF_WORKER_URL=https://your-bridge.workers.dev
SANDBOX_CF_API_TOKEN=<your-cloudflare-api-token>`}
            </pre>
            <p className="text-xs text-gray-500 dark:text-gray-500">
              Deploy the Bridge Worker:{" "}
              <a
                href="https://deploy.workers.cloudflare.com/?url=https://github.com/superplanehq/cloudflare-bridge"
                target="_blank"
                rel="noopener noreferrer"
                className="text-sky-600 dark:text-sky-400 hover:underline"
              >
                Deploy to Cloudflare Workers ↗
              </a>
            </p>
          </div>
        }
      />
    </div>
  );
}
