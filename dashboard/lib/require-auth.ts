import { auth } from "@/lib/auth";

export async function requireSession(
  request: Request,
  message: string,
): Promise<Response | null> {
  const session = await auth.api.getSession({
    headers: request.headers,
  });

  if (!session) {
    return Response.json({ error: message }, { status: 401 });
  }

  return null;
}

export function controlPlaneAuthHeaders(
  extra?: Record<string, string>,
): Record<string, string> {
  const headers: Record<string, string> = { ...extra };
  const token = process.env.SKYSCALE_DASHBOARD_TOKEN;
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

export function isGpuSpendRequest(body: string): boolean {
  try {
    const parsed = JSON.parse(body) as {
      hardware_type?: string;
      job_type?: string;
    };
    return (
      parsed.hardware_type === "gpu" || parsed.job_type === "training_run"
    );
  } catch {
    return false;
  }
}

/** Browser-side RL run start/stop paths proxied through /api/rl/[...path]. */
export function isRlRunSpendMethod(
  method: string,
  path: string,
): boolean {
  if (method === "POST" && path === "runs") {
    return true;
  }
  if (method === "DELETE" && /^runs\/[^/]+$/.test(path)) {
    return true;
  }
  return false;
}
