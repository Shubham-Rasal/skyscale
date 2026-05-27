import {
  controlPlaneAuthHeaders,
  requireSession,
} from "@/lib/require-auth";

export const runtime = "nodejs";

const CONTROL_PLANE_URL =
  process.env.SKYSCALE_CONTROL_PLANE_URL ??
  process.env.NEXT_PUBLIC_API_URL ??
  "http://localhost:8080";

export async function POST(request: Request) {
  const denied = await requireSession(
    request,
    "Sign in to launch GPU training jobs.",
  );
  if (denied) return denied;

  const body = await request.text();
  const upstream = await fetch(`${CONTROL_PLANE_URL}/api/training/jobs`, {
    method: "POST",
    headers: controlPlaneAuthHeaders({
      "Content-Type": request.headers.get("content-type") ?? "application/json",
    }),
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
