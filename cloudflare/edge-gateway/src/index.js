const securityHeaders = {
  "referrer-policy": "strict-origin-when-cross-origin",
  "strict-transport-security": "max-age=31536000; includeSubDomains",
  "x-content-type-options": "nosniff",
  "x-frame-options": "SAMEORIGIN",
};

const authRateLimitedPaths = new Set([
  "/api/user/login",
  "/api/user/login/2fa",
  "/api/user/passkey/login/begin",
  "/api/user/passkey/login/finish",
  "/api/user/register",
  "/api/user/reset",
]);

const emailRateLimitedPaths = new Set(["/api/reset_password", "/api/verification"]);

export default {
  async fetch(request, env) {
    const incomingUrl = new URL(request.url);

    if (incomingUrl.pathname === "/api/setup") {
      return new Response("Not Found", {
        status: 404,
        headers: {
          ...securityHeaders,
          "cache-control": "no-store",
        },
      });
    }

    if (request.method !== "OPTIONS") {
      const clientIp = request.headers.get("cf-connecting-ip") || "unknown";
      let rateLimitResult;

      if (authRateLimitedPaths.has(incomingUrl.pathname)) {
        rateLimitResult = await env.AUTH_RATE_LIMITER.limit({
          key: `${incomingUrl.pathname}:${clientIp}`,
        });
      } else if (emailRateLimitedPaths.has(incomingUrl.pathname)) {
        rateLimitResult = await env.EMAIL_RATE_LIMITER.limit({
          key: `${incomingUrl.pathname}:${clientIp}`,
        });
      }

      if (rateLimitResult && !rateLimitResult.success) {
        return new Response("Too Many Requests", {
          status: 429,
          headers: {
            ...securityHeaders,
            "cache-control": "no-store",
            "retry-after": "60",
          },
        });
      }
    }

    const originUrl = new URL(env.ORIGIN_URL);
    const originBaseUrl = originUrl.origin;
    originUrl.pathname = incomingUrl.pathname;
    originUrl.search = incomingUrl.search;

    const originRequest = new Request(originUrl, request);
    originRequest.headers.set("x-edge-origin-auth", env.ORIGIN_AUTH_SECRET);
    originRequest.headers.set("x-forwarded-host", incomingUrl.host);
    originRequest.headers.set("x-forwarded-proto", "https");

    const originResponse = await fetch(originRequest);
    const responseHeaders = new Headers(originResponse.headers);
    const location = responseHeaders.get("location");

    if (location?.startsWith(originBaseUrl)) {
      responseHeaders.set("location", location.replace(originBaseUrl, incomingUrl.origin));
    }

    responseHeaders.delete("server");
    responseHeaders.delete("x-powered-by");
    for (const [name, value] of Object.entries(securityHeaders)) {
      responseHeaders.set(name, value);
    }

    return new Response(originResponse.body, {
      status: originResponse.status,
      statusText: originResponse.statusText,
      headers: responseHeaders,
    });
  },
};
