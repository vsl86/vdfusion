# VDFusion

VDFusion is a powerful, high-performance video duplicate finder that uses deep content analysis to identify identical or near-identical video files, even if they have different names, resolutions, or bitrates.

It provides both a **Native Desktop Experience** (powered by Wails) and a **Headless Server** with a Web UI for NAS and remote server deployments.

## Core Features

- **Deep Analysis**: Uses perceptual hashing (pHash) and metadata probing to find duplicates accurately.
- **FFmpeg Powered**: Uses FFmpeg for media handling and "suspicious" file detection (detects corruption and stream errors, suggest ffmpeg commands to fix them).
- **Activity Log**: Real-time monitoring of scan progress, phase results, and file deletions.
- **Portable**: macOS builds are self-contained with bundled FFmpeg dependencies.
- **Fast**: Concurrent scanning handles large libraries with thousands of videos efficiently (it really depends on speed of your storage, but app works as quick as possible).
- **Headless Server with Web UI**: Easy deployment on NAS (Unraid, Synology, Casa/ZimaOS etc.) with Web UI (anywhere with docker).

## Installation & Setup

### Desktop (macOS & Windows)

1. Download the latest `.dmg` (macOS) or `.exe` (Windows/*soon*) from the [Releases](https://github.com/vsl86/vdfusion/releases) page.
2. **macOS Users**: Since the app is currently unsigned, you will see a warning. To open it:
    - run app (you'll get warning).
    - go to "settings" -> "privacy & security" -> scroll to bottom -> click "open anyway" on VDFusion.

### Docker (Headless Server)

The headless version is perfect for running on a continuous server. Use the following `docker-compose.yml` to get started:

```yaml
services:
  vdfusion:
    image: vsl86/vdfusion:v0.9.9
    ports:
      - "8080:8080"
    volumes:
      - /opt/vdfusion/storage:/app/storage ### persistent storage for DB, configs and logs
      - /your/media/dir:/media ### mount of your media directory
    environment:
      - VDF_DB_PATH=/app/storage/vdf.db
      - VDF_SERVER_ADDR=:8080
      - VDF_LOG_LEVEL=info
    restart: unless-stopped
```

1. Run `docker-compose up -d`.
2. Access the UI at `http://your-server-ip:8080`.

## Key Usage Tips
- **Scanning**: Add folders to scan, setup scan parameters (similarity level, duration difference, etc.), and start scanning. 
- **Important**: Don't set similarity level too low, it will produce too many false positives, start with 95 and go lower if needed. first scan will take some time, app will show estimations.
- **Activity Log**: Click the "Activity Log" button in the status bar to see a history of what the app is doing (e.g., how many files were scanned, which files were deleted).
- **Debug Logging**: If you experience issues or want to see exactly which files are being scanned, enable "Debug Logging" in the scan settings.

## Contributing & Development

Contributions are welcome! If you want to build from source, set up a development environment, or learn about the architecture, please see our [Development Guide](docs/DEVELOPMENT.md).

## Credits

- **Video Duplicate Finder**: [https://github.com/0x90d/videoduplicatefinder](https://github.com/0x90d/videoduplicatefinder)
- **FFmpeg**: [https://ffmpeg.org/](https://ffmpeg.org/)
- **Wails**: [https://wails.io/](https://wails.io/)
- **pHash**: [https://phash.org/](https://phash.org/)
---
*Created with ❤️ by vsl86*
