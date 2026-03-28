import { createWorker } from "@cloudflare/worker-bundler";

export interface Env {
  LOADER: WorkerLoader;
  AUTH_TOKEN: string;
}

interface ExecuteRequest {
  code: string;
  language: string;
  input: Record<string, unknown>;
  timeoutMs?: number;
}

interface ExecuteResponse {
  output: Record<string, unknown>;
  logs: string[];
  exitCode: number;
  durationMs: number;
  error?: string;
}

const LANGUAGE_TEMPLATES: Record<string, (code: string, input: string) => string> = {
  javascript: (code, input) => `
const SUPERPLANE_INPUT = ${input};
${code}
`,
  typescript: (code, input) => `
const SUPERPLANE_INPUT: Record<string, unknown> = ${input};
${code}
`,
  python: (_code, _input) => {
    throw new Error("Python requires Pyodide WASM — use gVisor provider for Python");
  },
};

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (request.method === "GET" && new URL(request.url).pathname === "/health") {
      return new Response("ok", { status: 200 });
    }

    if (request.method === "GET" && new URL(request.url).pathname === "/status") {
      return Response.json({ available: true, provider: "cloudflare-dynamic-workers" });
    }

    if (request.method !== "POST" || new URL(request.url).pathname !== "/execute") {
      return new Response("not found", { status: 404 });
    }

    const authHeader = request.headers.get("Authorization");
    if (!authHeader || authHeader !== `Bearer ${env.AUTH_TOKEN}`) {
      return new Response("unauthorized", { status: 401 });
    }

    let body: ExecuteRequest;
    try {
      body = await request.json<ExecuteRequest>();
    } catch {
      return new Response("invalid JSON body", { status: 400 });
    }

    if (!body.code) {
      return new Response("code is required", { status: 400 });
    }

    const language = (body.language || "javascript").toLowerCase();
    const template = LANGUAGE_TEMPLATES[language];
    if (!template) {
      return Response.json(
        { error: `unsupported language: ${language}. Supported: javascript, typescript` },
        { status: 400 },
      );
    }

    const inputJSON = JSON.stringify(body.input || {});
    let wrappedCode: string;
    try {
      wrappedCode = template(body.code, inputJSON);
    } catch (e) {
      return Response.json({ error: (e as Error).message }, { status: 400 });
    }

    const logs: string[] = [];
    const start = Date.now();

    try {
      const { mainModule, modules } = await createWorker({
        files: {
          "worker.js": wrappedCode,
        },
        bundle: true,
        minify: false,
      });

      const workerId = await hashCode(body.code);

      const worker = env.LOADER.get(workerId, async () => ({
        mainModule,
        modules,
      }));

      const workerRequest = new Request("https://sandbox.internal/run", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: inputJSON,
      });

      const workerResponse = await worker.getEntrypoint().fetch(workerRequest);
      const durationMs = Date.now() - start;

      if (!workerResponse.ok) {
        const errorText = await workerResponse.text();
        const result: ExecuteResponse = {
          output: {},
          logs,
          exitCode: 1,
          durationMs,
          error: errorText,
        };
        return Response.json(result);
      }

      let output: Record<string, unknown> = {};
      const responseText = await workerResponse.text();
      try {
        output = JSON.parse(responseText);
      } catch {
        output = { raw: responseText };
      }

      const result: ExecuteResponse = {
        output,
        logs,
        exitCode: 0,
        durationMs,
      };

      return Response.json(result);
    } catch (e) {
      const durationMs = Date.now() - start;
      const result: ExecuteResponse = {
        output: {},
        logs,
        exitCode: 1,
        durationMs,
        error: (e as Error).message,
      };
      return Response.json(result, { status: 500 });
    }
  },
};

async function hashCode(code: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(code);
  const hashBuffer = await crypto.subtle.digest("SHA-256", data);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("")
    .slice(0, 16);
}
