This is a [Next.js](https://nextjs.org) project bootstrapped with [`create-next-app`](https://nextjs.org/docs/app/api-reference/cli/create-next-app).

## Getting Started

First, run the development server:

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000). Training UI is at `/`; **FaaS Containers** is at **`/faas`**.

## FaaS Containers + Railway GraphQL

The `/faas` page deploys and stops workloads through Next.js API routes that call the [Railway Public GraphQL API](https://docs.railway.com/integrations/api) (`https://backboard.railway.com/graphql/v2`) using **`RAILWAY_API_TOKEN`** on the server only (`Authorization: Bearer …`).

### Environment variables

| Variable | Description |
|----------|-------------|
| `RAILWAY_API_TOKEN` | **Required** for FaaS. Account or workspace token from Railway. Do not commit; set in Railway service variables or `.env.local`. |
| `RAILWAY_GRAPHQL_URL` | Optional. Defaults to `https://backboard.railway.com/graphql/v2`. |
| `FAAS_RAILWAY_SERVICE_ID` | Default **service** id. If unset, **`RAILWAY_SERVICE_ID`** is used (injected when this app runs on Railway). |
| `FAAS_RAILWAY_ENVIRONMENT_ID` | Default **environment** id. If unset, **`RAILWAY_ENVIRONMENT_ID`** is used (injected on Railway). |
| `FAAS_RAILWAY_PROJECT_ID` | Optional for deployment list queries. If unset, **`RAILWAY_PROJECT_ID`** is used when present. |
| `FAAS_RAILWAY_SERVICES_JSON` | Optional JSON map: template id → `{ "serviceId", "environmentId", "projectId?" }` for different services per template. |

Template ids are in [`lib/faas/templates.ts`](lib/faas/templates.ts).

**Local dev:** Railway does not inject IDs into `npm run dev`. Set `FAAS_RAILWAY_SERVICE_ID` and `FAAS_RAILWAY_ENVIRONMENT_ID` (Cmd/Ctrl+K in Railway to copy).

**Hosted on Railway:** Often only `RAILWAY_API_TOKEN` is needed; binding falls back to injected `RAILWAY_SERVICE_ID` / `RAILWAY_ENVIRONMENT_ID`. Use `FAAS_*` or JSON map if “Spin up” should target a **different** service than the dashboard.

### Deploy on Railway

1. Create a token at [railway.com/account/tokens](https://railway.com/account/tokens).
2. Set **`RAILWAY_API_TOKEN`** on the dashboard service.
3. Either rely on injected **`RAILWAY_SERVICE_ID` / `RAILWAY_ENVIRONMENT_ID`**, or set **`FAAS_RAILWAY_*`** / **`FAAS_RAILWAY_SERVICES_JSON`** for a separate worker service.
4. Deploy and open `/faas`.

## Learn More

- [Next.js Documentation](https://nextjs.org/docs)
- [Railway API cookbook](https://docs.railway.com/integrations/api/api-cookbook)
