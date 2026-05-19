import { auth } from "@/lib/auth";

export const runtime = "nodejs";

const CONTROL_PLANE_URL =
  process.env.SKYSCALE_CONTROL_PLANE_URL ??
  process.env.NEXT_PUBLIC_API_URL ??
  "http://localhost:8080";

export async function POST(request: Request) {
  const session = await auth.api.getSession({
    headers: request.headers,
  });

  if (!session) {
    return Response.json(
      { error: "Sign in to launch GPU training jobs." },
      { status: 401 },
    );
  }

  const body = await request.text();
  const upstream = await fetch(`${CONTROL_PLANE_URL}/api/training/jobs`, {
    method: "POST",
    headers: {
      "Content-Type": request.headers.get("content-type") ?? "application/json",
    },
    body,
    cache: "no-store",
  });

  return new Response(upstream.body, {
    status: upstream.status,
    headers: {
      "Content-Type": upstream.headers.get("content-type") ?? "application/json",
    },
  });
}
