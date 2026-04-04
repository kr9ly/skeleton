import { Router } from "express";
import jwt from "jsonwebtoken";
import type { Config } from "./config/env";

export interface JwtPayload {
  sub: string;
  role: Role;
  iat: number;
}

export type Role = "admin" | "user";

export function verifyToken(token: string): Promise<JwtPayload> {
  const decoded = jwt.verify(token, SECRET);
  return { sub: decoded.sub, role: decoded.role, iat: decoded.iat };
}

export const signToken = (payload: Record<string, unknown>, expiresIn?: string): string => {
  return jwt.sign(payload, SECRET, { expiresIn: expiresIn ?? "1h" });
};

export class AuthService {
  private readonly secret: string;

  constructor(config: Config) {
    this.secret = config.jwtSecret;
  }

  async verify(token: string): Promise<JwtPayload> {
    return verifyToken(token);
  }
}

export default function createRouter(): Router {
  const router = Router();
  router.get("/health", (_, res) => res.json({ ok: true }));
  return router;
}

const internalHelper = () => "not exported";
