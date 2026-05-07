// Posts
export interface SosPost {
  id: string
  name: string
  kmMarker: string
  phoneNumber: string
  status: "OK" | "FAIL" | "UNKNOWN" | "TESTING"
  disabled: boolean
  lastTestDate?: string | null
  CreatedAt: string
  UpdatedAt: string
}

export interface PostPatch {
  name?: string
  kmMarker?: string
  phoneNumber?: string
  status?: SosPost["status"]
  disabled?: boolean
}

export interface PostsSummary {
  total: number
  byStatus: Record<string, number>
}

// Tests
export type TestType = "CONNECTIVITY" | "SMS" | "AUDIO_MIC" | "AUDIO_SPEAKER"

// cloud-gesvial.20 dual-evidence classification subtypes. See manual sec 6.3.
//   POST_NOT_RESPONDING — delivery confirmed by carrier, post did not reply
//                         (escalate: field team, physical module).
//   NETWORK_DELIVERY    — no carrier receipt, no post reply (transport
//                         failure: gateway / coverage / SIM).
//   TIMEOUT             — no evidence at all in the test window.
export type FailureKind = "POST_NOT_RESPONDING" | "NETWORK_DELIVERY" | "TIMEOUT"

export interface TestResult {
  id: string
  deviceId?: string | null
  postId: string
  testType: TestType
  status: "PENDING" | "PASSED" | "FAILED" | "ERROR"
  callRecordId?: string | null
  messageId?: string | null
  fftAnalysisJson?: string | null
  details?: string | null
  error?: string | null
  startedAt?: string | null
  completedAt?: string | null
  // cloud-gesvial.20 dual-evidence classification. NULL for legacy rows
  // (pre-20260501) and for tests still in PENDING. The panel renders "—" for
  // null transport-layer fields. See manual-operador.md §6.3.
  deliveryConfirmed?: boolean | null
  deliveryAt?: string | null
  deliveryCarrier?: string | null
  failureKind?: FailureKind | null
  CreatedAt: string
  UpdatedAt: string
}

export interface BatchScheduleItem {
  postId: string
  testResultId: string
  status: "PENDING" | "PENDING_EXISTING" | "ERROR"
  error?: string
}

export interface BatchScheduleResponse {
  results: BatchScheduleItem[]
}

// Test schedules (gesvial.11; cancelPending added in cloud-gesvial.17)
export interface TestSchedule {
  id: string
  userId: string
  name: string
  cronExpression: string
  testType: TestType
  enabled: boolean
  onlyEnabledPosts: boolean
  // cloud-gesvial.17: when true, the cron dispatcher cancels any in-flight
  // PENDING for the same (post, testType) BEFORE re-scheduling. Default
  // false preserves legacy dedupe behaviour. Useful when the operator wants
  // the cron to ALWAYS run on time regardless of stuck or slow gateways.
  cancelPending: boolean
  filterPostIds?: string[] | null
  deviceId?: string | null
  lastRunAt?: string | null
  CreatedAt: string
  UpdatedAt: string
}

export interface TestScheduleInput {
  name: string
  cronExpression: string
  testType: TestType
  enabled: boolean
  onlyEnabledPosts: boolean
  cancelPending: boolean
  filterPostIds?: string[] | null
  deviceId?: string | null
}

// Devices
export interface Device {
  id: string
  name: string
  createdAt: string
  updatedAt: string
  lastSeen: string
  // cloud-gesvial.19.3 D7: identity fields. El gateway los reporta en
  // /api/mobile/v1/heartbeat. NULL hasta que la app de la versión nueva
  // haya hecho su primer heartbeat post-deploy.
  phoneNumber?: string | null
  model?: string | null
  osVersion?: string | null
  // hasPushToken indica si el cloud puede contactar al device por FCM.
  // Si false → la única vía es SSE (la conexión HTTP persistente que el
  // device abre al cloud). Útil para diagnosticar "el cloud envió un
  // test pero no llegó al Z5".
  hasPushToken?: boolean
}

// Auth
export interface TokenResponse {
  id: string
  token_type: string
  access_token: string
  refresh_token: string
  expires_at: string
}

export interface TokenRequest {
  scopes: string[]
  ttl: number
}
