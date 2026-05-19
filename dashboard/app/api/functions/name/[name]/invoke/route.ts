export const runtime = "nodejs";

const CONTROL_PLANE_URL =
  process.env.SKYSCALE_CONTROL_PLANE_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  "http://localhost:8080";

type RouteContext = {
  params: Promise<{ name: string }>;
};

export async function POST(request: Request, context: RouteContext) {
  const { name } = await context.params;

  try {
    const body = await request.text();
    const upstream = await fetch(
      `${CONTROL_PLANE_URL}/api/functions/name/${encodeURIComponent(name)}/invoke`,
      {
        method: "POST",
        headers: {
          "Content-Type": request.headers.get("content-type") ?? "application/json",
        },
        body,
        cache: "no-store",
      },
    );

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
