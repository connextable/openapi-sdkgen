export interface GeneratedArtifact {
  path: string;
  content: string;
}

export interface GenerationResult {
  artifacts?: GeneratedArtifact[];
  diagnostics?: string;
  error?: string;
}

interface GenerationRequest {
  id: number;
  source: string;
  target: string;
}

interface GenerationResponse {
  id: number;
  result?: GenerationResult;
  error?: string;
}

interface PendingGeneration {
  resolve: (result: GenerationResult) => void;
  reject: (error: Error) => void;
}

let generatorWorker: Worker | undefined;
let nextRequestID = 0;
const pendingGenerations = new Map<number, PendingGeneration>();

function rejectPending(error: Error) {
  pendingGenerations.forEach(({ reject }) => reject(error));
  pendingGenerations.clear();
  generatorWorker?.terminate();
  generatorWorker = undefined;
}

function worker(): Worker {
  if (generatorWorker) return generatorWorker;

  generatorWorker = new Worker(`${import.meta.env.BASE_URL}playground/generator-worker.js`);
  generatorWorker.addEventListener("message", (event: MessageEvent<GenerationResponse>) => {
    const pending = pendingGenerations.get(event.data.id);
    if (!pending) return;
    pendingGenerations.delete(event.data.id);
    if (event.data.error) {
      pending.reject(new Error(event.data.error));
      return;
    }
    pending.resolve(event.data.result ?? {});
  });
  generatorWorker.addEventListener("error", () => {
    rejectPending(new Error("Generator worker stopped unexpectedly."));
  });
  return generatorWorker;
}

export function generate(source: string, target: string): Promise<GenerationResult> {
  return new Promise((resolve, reject) => {
    const id = nextRequestID;
    nextRequestID += 1;
    pendingGenerations.set(id, { resolve, reject });
    const request: GenerationRequest = { id, source, target };
    worker().postMessage(request);
  });
}
