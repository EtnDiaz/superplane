# SuperPlane Sandbox Bridge Worker

Cloudflare Worker that executes arbitrary JS/TS code in isolated Dynamic Worker sandboxes.

## Deploy

```bash
cd cloudflare-bridge
npm install
npm run deploy
```

## Configure

After deploy, set your `AUTH_TOKEN` secret:

```bash
wrangler secret put AUTH_TOKEN
```

Then in SuperPlane canvas settings:
- **Sandbox Runtime** → `Cloudflare Dynamic Workers`
- **Bridge URL** → `https://superplane-sandbox-bridge.<your-subdomain>.workers.dev`
- **Auth Token** → your secret token

## API

### `POST /execute`

Headers: `Authorization: Bearer <AUTH_TOKEN>`

```json
{
  "code": "export default { async fetch(req) { return Response.json({ result: 42 }); } }",
  "language": "javascript",
  "input": { "key": "value" },
  "timeoutMs": 30000
}
```

Response:
```json
{
  "output": { "result": 42 },
  "logs": [],
  "exitCode": 0,
  "durationMs": 12
}
```

### `GET /health` — returns `200 ok`
### `GET /status` — returns `{ "available": true }`

## How it works

1. SuperPlane calls `POST /execute` with user code + input
2. Bridge Worker bundles code via `@cloudflare/worker-bundler`
3. Loads into `worker_loaders` binding (cached by SHA-256 of code)
4. Invokes the dynamic worker with input as request body
5. Returns JSON output + logs

## Input access in user code

```javascript
export default {
  async fetch(request) {
    const input = await request.json(); // SUPERPLANE_INPUT
    const result = { processed: input.items?.length ?? 0 };
    return Response.json(result);
  }
}
```
