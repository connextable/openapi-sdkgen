let runtimePromise;

function assetURL(name) {
  return new URL(name, self.location.href).href;
}

async function startRuntime() {
  self.importScripts(assetURL("wasm_exec.js"));
  if (!self.Go) throw new Error("Go WebAssembly runtime is unavailable.");

  const go = new self.Go();
  const response = await fetch(assetURL("openapi-sdkgen.wasm"));
  if (!response.ok) throw new Error(`Could not load the generator (${response.status}).`);
  const bytes = await response.arrayBuffer();
  const module = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(module.instance);

  for (let attempt = 0; attempt < 100 && !self.openapiSDKGenGenerate; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  if (!self.openapiSDKGenGenerate) throw new Error("Generator did not finish starting.");
}

self.addEventListener("message", async (event) => {
  const response = { id: event.data.id };
  try {
    runtimePromise ??= startRuntime();
    await runtimePromise;
    if (!self.openapiSDKGenGenerate) throw new Error("Generator is unavailable.");
    response.result = JSON.parse(self.openapiSDKGenGenerate(event.data.source, event.data.target));
  } catch (cause) {
    response.error = cause instanceof Error ? cause.message : String(cause);
  }
  self.postMessage(response);
});
