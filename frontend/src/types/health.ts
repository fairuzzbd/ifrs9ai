/**
 * Bentuk respons endpoint backend `GET /healthz`.
 * Selaras dengan handler health-check di backend Go (Gin).
 */
export interface HealthResponse {
  status: string;
  service: string;
  version: string;
  timestamp: string;
}
