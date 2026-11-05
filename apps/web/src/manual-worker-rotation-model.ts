import type {ReleaseWorkerRotationResult} from '../../../packages/contracts/src';
import type {ToolPendingRequest} from './tool-write';

const rotationPath = /^\/release-workers\/([a-f0-9]{32})\/rotate-credential$/;
const publicWorkerID = /^[A-Za-z0-9_.-]{1,100}$/;
const digest = /^[a-f0-9]{64}$/;
const maxRotationVersion = 2_147_483_646;

export type ManualWorkerRotationReceipt = ReleaseWorkerRotationResult;

export function manualWorkerRotationID(path: string) {
  return rotationPath.exec(path)?.[1];
}

export function validManualWorkerRotationCommand(
  path: string,
  body: unknown,
  method: string,
  project: string | undefined,
) {
  return !!manualWorkerRotationID(path) && method === 'POST' && !!project &&
    object(body) && Object.keys(body).length === 3 &&
    body.project_id === project && body.acknowledge_stopped === true &&
    typeof body.expected_version === 'number' && Number.isInteger(body.expected_version) &&
    body.expected_version >= 1 && body.expected_version <= maxRotationVersion;
}

export function validManualWorkerRotationOperation(key: unknown): key is string {
  return typeof key === 'string' && /^[A-Za-z0-9_.-]{8,128}$/.test(key);
}

export function validManualWorkerRotationReceipt(
  value: unknown,
  request: ToolPendingRequest,
  project: string,
): value is ManualWorkerRotationReceipt {
  if (!validManualWorkerRotationCommand(request.path, request.body, request.method, project) ||
    !validManualWorkerRotationOperation(request.idempotency_key) || !object(value) ||
    value.operation_id !== request.idempotency_key ||
    Object.keys(value).some(key => !['operation_id', 'worker', 'token'].includes(key)) ||
    (Object.hasOwn(value, 'token') && (typeof value.token !== 'string' || !digest.test(value.token)))) {
    return false;
  }
  const worker = value.worker;
  const version = Number(request.body.expected_version) + 1;
  const allowsRotation = version <= maxRotationVersion;
  return object(worker) && Object.keys(worker).length === (allowsRotation ? 10 : 9) &&
    worker.id === manualWorkerRotationID(request.path) && worker.project_id === project &&
    typeof worker.worker_id === 'string' && publicWorkerID.test(worker.worker_id) &&
    worker.version === version && worker.status === 'active' &&
    (allowsRotation ? worker.manual_rotation_allowed === true : !Object.hasOwn(worker, 'manual_rotation_allowed')) &&
    typeof worker.deployment === 'boolean' &&
    validTime(worker.created_at) && validTime(worker.expires_at) && validProfiles(worker.profiles);
}

function object(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function validTime(value: unknown) {
  return typeof value === 'string' && Number.isFinite(Date.parse(value));
}

function validProfiles(value: unknown) {
  return value === null || Array.isArray(value) && value.length <= 32 && value.every(profile =>
    object(profile) && Object.keys(profile).length === 3 &&
    typeof profile.id === 'string' && publicWorkerID.test(profile.id) &&
    (profile.kind === 'build' || profile.kind === 'package_test') &&
    typeof profile.sha256 === 'string' && digest.test(profile.sha256),
  );
}
