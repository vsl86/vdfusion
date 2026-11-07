1. low batching for neural processing - on modern hw it takes 50ms to process all images from single video, so ANE/NPU is waiting most of time. We should use larger batches

2. DB grow. embeddings are stored in sqlite db as BLOBs and it grow fast. We should use more efficient storage. Also vector search is slow. it's time to explore https://github.com/asg017/sqlite-vec

3. slowed down comparison phase. ~7-10 times slower. probably related to #2

