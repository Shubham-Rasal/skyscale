export const runtime = "nodejs";

const CONTROL_PLANE_URL =
  process.env.SKYSCALE_CONTROL_PLANE_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  "http://localhost:8080";

async function proxyFunctions(request: Request, method: "GET" | "POST") {
  try {
    const body = method === "POST" ? await request.text() : undefined;
    const upstream = await fetch(`${CONTROL_PLANE_URL}/api/functions`, {
      method,
      headers: method === "POST"
        ? { "Content-Type": request.headers.get("content-type") ?? "application/json" }
        : undefined,
      body,
      cache: "no-store",
    });

    return new Response(upstream.body, {
      status: upstream.status,
      headers: {
        "Content-Type": upstream.headers.get("content-type") ?? "application/json",
      },
    });
  } catch (error) {
    return Response.json(
      {
        error: `Could not reach control plane at ${CONTROL_PLANE_URL}: ${error instanceof Error ? error.message : "unknown error"}`,
      },
      { status: 502 },
    );
  }
}

export async function GET(request: Request) {
  return proxyFunctions(request, "GET");
}

export async function POST(request: Request) {
  return proxyFunctions(request, "POST");
}
