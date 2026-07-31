"""
CoreML Process Worker — runs CoreML inference in a dedicated child process
so that macOS GCD dispatch calls inside coremltools/libcoreml run on a clean
main thread without deadlocking against Uvicorn's asyncio event loop.
"""

from __future__ import annotations

import multiprocessing as mp
import queue
import traceback
from pathlib import Path
import numpy as np


def _coreml_worker_loop(
    model_path_str: str,
    compute_units: int,
    compiled_batch_size: int,
    input_queue: mp.Queue,
    output_queue: mp.Queue,
) -> None:
    import coremltools as ct
    from coremltools.models import CompiledMLModel
    import psutil

    print(f"[coreml-worker] Loading CoreML model from {model_path_str}…")
    path = Path(model_path_str)
    
    # Define compute units to try (in order of priority)
    if compute_units is not None:
        compute_units_list = [compute_units]
    else:
        compute_units_list = [
            ct.ComputeUnit.CPU_AND_NE.value,
            ct.ComputeUnit.CPU_AND_GPU.value,
            ct.ComputeUnit.CPU_ONLY.value,
        ]
    
    model = None
    output_name = "image_embeds"
    for cu in compute_units_list:
        try:
            print(f"[coreml-worker] Trying with compute units: {ct.ComputeUnit(cu)}")
            
            mem = psutil.virtual_memory()
            print(f"[coreml-worker] System memory: {mem.total / (1024**3):.1f} GB total, {mem.available / (1024**3):.1f} GB available")
            
            print(f"[coreml-worker] Model path exists: {path.exists()}, suffix: {path.suffix}")
            
            if path.suffix == ".mlmodelc":
                model = CompiledMLModel(str(path), compute_units=ct.ComputeUnit(cu))
                print("[coreml-worker] Loaded CompiledMLModel")
            else:
                model = ct.models.MLModel(str(path), compute_units=ct.ComputeUnit(cu))
                print(f"[coreml-worker] Loaded MLModel from {path.suffix}")
                print(f"[coreml-worker] CoreML model input features: {model.input_description}")
                print(f"[coreml-worker] CoreML model output features: {model.output_description}")
                
            print(f"[coreml-worker] CoreML model loaded cleanly; warming up with batch size {compiled_batch_size}…")
            dummy = np.zeros((compiled_batch_size, 3, 224, 224), dtype=np.float32)
            warmup_res = model.predict({"pixel_values": dummy})
            output_name = "image_embeds" if "image_embeds" in warmup_res else list(warmup_res.keys())[0]
            print(f"[coreml-worker] Warmup complete, output={output_name}, shape={warmup_res[output_name].shape}")
            print(f"[coreml-worker] Ready for inference (compute_units={cu}).")
            break  # Exit loop if we successfully loaded and warmed up the model
        except Exception as exc:
            print(f"[coreml-worker] Failed to load with compute units {ct.ComputeUnit(cu)}: {exc}")
            print(f"[coreml-worker] Traceback: {traceback.format_exc()}")
            continue  # Try next compute unit
    
    if model is None:
        output_queue.put(("INIT_ERROR", "Failed to load CoreML model with all available compute units"))
        return

    output_queue.put(("INIT_OK", {"input_name": "pixel_values", "output_name": output_name}))

    while True:
        item = input_queue.get()
        if item is None:
            break
        req_id, batch_arr = item
        try:
            print(f"[coreml-worker] Received request {req_id} with batch shape: {batch_arr.shape}")
            res = model.predict({"pixel_values": batch_arr})
            # CoreML outputs dictionary mapping output feature name -> numpy array
            output_name = "image_embeds" if "image_embeds" in res else list(res.keys())[0]
            print(f"[coreml-worker] Request {req_id} complete, output shape: {res[output_name].shape}")
            output_queue.put((req_id, res[output_name]))
        except Exception as exc:
            print(f"[coreml-worker] Request error: {exc}")
            print(f"[coreml-worker] Traceback: {traceback.format_exc()}")
            output_queue.put((req_id, f"{exc}\n{traceback.format_exc()}"))


class CoreMLProcessBridge:
    def __init__(
        self,
        model_path: Path,
        compute_units: int | None = None,
        compiled_batch_size: int = 1,
        startup_timeout: float = 60.0,
        predict_timeout: float = 60.0,
    ):
        import sys
        if sys.platform != "darwin":
            raise RuntimeError("CoreML is only available on macOS")
            
        import coremltools as ct

        self.model_path = model_path
        
        # Keep CPU_AND_NE (ANE) as default; allow user to override
        if compute_units is None:
            compute_units = ct.ComputeUnit.CPU_AND_NE.value

        ctx = mp.get_context("spawn")
        self.input_queue = ctx.Queue()
        self.output_queue = ctx.Queue()
        self.process = ctx.Process(
            target=_coreml_worker_loop,
            args=(str(model_path), compute_units, compiled_batch_size, self.input_queue, self.output_queue),
            daemon=True,
        )
        self.process.start()

        self.predict_timeout = predict_timeout

        # Wait for load + first predict. If CoreML hangs, kill the child so the
        # parent server can fall back instead of hanging indefinitely.
        try:
            status, payload = self.output_queue.get(timeout=startup_timeout)
        except queue.Empty as exc:
            self._kill_worker()
            raise RuntimeError(
                f"CoreML worker timed out during load/warmup after {startup_timeout:.0f}s"
            ) from exc
        if status != "INIT_OK":
            self._kill_worker()
            raise RuntimeError(f"CoreML worker initialization failed: {payload}")

        self._input_names = [payload.get("input_name", "pixel_values")] if isinstance(payload, dict) else ["pixel_values"]
        self._output_names = [payload.get("output_name", "image_embeds")] if isinstance(payload, dict) else ["image_embeds"]
        self._req_counter = 0

    def predict(self, batch_arr: np.ndarray) -> np.ndarray:
        self._req_counter += 1
        req_id = self._req_counter
        self.input_queue.put((req_id, batch_arr))
        try:
            _res_id, result = self.output_queue.get(timeout=self.predict_timeout)
        except queue.Empty as exc:
            self._kill_worker()
            raise RuntimeError(
                f"CoreML worker timed out during prediction after {self.predict_timeout:.0f}s"
            ) from exc
        if isinstance(result, Exception):
            raise result
        if isinstance(result, str):
            raise RuntimeError(result)
        return result

    def _kill_worker(self):
        try:
            if self.process.is_alive():
                self.process.kill()
            self.process.join(timeout=2)
        except Exception:
            pass

    def close(self):
        try:
            self.input_queue.put(None)
            self.process.join(timeout=2)
        except Exception:
            pass
