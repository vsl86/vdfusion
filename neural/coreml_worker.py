"""
CoreML Process Worker — runs CoreML inference in a dedicated child process
so that macOS GCD dispatch calls inside coremltools/libcoreml run on a clean
main thread without deadlocking against Uvicorn's asyncio event loop.
"""

from __future__ import annotations

import multiprocessing as mp
import time
import traceback
from pathlib import Path
import numpy as np


def _coreml_worker_loop(model_path_str: str, compute_units: int, input_queue: mp.Queue, output_queue: mp.Queue) -> None:
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
                
            print(f"[coreml-worker] CoreML model loaded cleanly! Ready for inference (compute_units={cu}).")
            break  # Exit loop if we successfully loaded the model
        except Exception as exc:
            print(f"[coreml-worker] Failed to load with compute units {ct.ComputeUnit(cu)}: {exc}")
            print(f"[coreml-worker] Traceback: {traceback.format_exc()}")
            continue  # Try next compute unit
    
    if model is None:
        output_queue.put(("INIT_ERROR", "Failed to load CoreML model with all available compute units"))
        return

    output_queue.put(("INIT_OK", None))

    while True:
        try:
            item = input_queue.get()
            if item is None:
                break
            req_id, batch_arr = item
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
    def __init__(self, model_path: Path, compute_units: int | None = None):
        import sys
        if sys.platform != "darwin":
            raise RuntimeError("CoreML is only available on macOS")
            
        import coremltools as ct
        import psutil

        self.model_path = model_path
        
        # Keep CPU_AND_NE (ANE) as default; allow user to override
        if compute_units is None:
            compute_units = ct.ComputeUnit.CPU_AND_NE.value

        ctx = mp.get_context("spawn")
        self.input_queue = ctx.Queue()
        self.output_queue = ctx.Queue()
        self.process = ctx.Process(
            target=_coreml_worker_loop,
            args=(str(model_path), compute_units, self.input_queue, self.output_queue),
            daemon=True,
        )
        self.process.start()

        # Wait for initialization status
        status, err = self.output_queue.get(timeout=30)
        if status != "INIT_OK":
            raise RuntimeError(f"CoreML worker initialization failed: {err}")

        self._req_counter = 0

    def predict(self, batch_arr: np.ndarray) -> np.ndarray:
        self._req_counter += 1
        req_id = self._req_counter
        self.input_queue.put((req_id, batch_arr))
        res_id, result = self.output_queue.get(timeout=30)
        if isinstance(result, Exception):
            raise result
        return result

    def close(self):
        try:
            self.input_queue.put(None)
            self.process.join(timeout=2)
        except Exception:
            pass
