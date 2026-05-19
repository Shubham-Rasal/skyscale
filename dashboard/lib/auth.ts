import { betterAuth } from "better-auth";
import { nextCookies } from "better-auth/next-js";
import { Pool } from "pg";

const databaseUrl = process.env.DATABASE_URL;

export const auth = betterAuth({
  baseURL: process.env.BETTER_AUTH_URL ?? process.env.NEXT_PUBLIC_APP_URL ?? "http://localhost:3000",
  secret: process.env.BETTER_AUTH_SECRET ?? "dev-only-change-this-secret-before-production",
  database: databaseUrl
    ? new Pool({
        connectionString: databaseUrl,
        ssl: process.env.NODE_ENV === "production" ? { rejectUnauthorized: false } : undefined,
      })
    : undefined,
  emailAndPassword: {
    enabled: true,
  },
  trustedOrigins: [
    process.env.BETTER_AUTH_URL,
    process.env.NEXT_PUBLIC_APP_URL,
  ].filter((origin): origin is string => Boolean(origin)),
  plugins: [nextCookies()],
});
