# VDFusion

VDFusion is a powerful, high-performance video duplicate finder that uses deep content analysis to identify identical or near-identical video files, even if they have different names, resolutions, or bitrates.

It provides both a **Native Desktop Experience** (powered by Wails) and a **Headless Server** with a Web UI for NAS and remote server deployments.

## Core Features

- **Deep Analysis**: Uses perceptual hashing (pHash) and metadata probing to find duplicates accurately.
- **FFmpeg Powered**: Uses FFmpeg for media handling and "suspicious" file detection (detects corruption and stream errors).
- **Activity Log**: Real-time monitoring of scan progress, phase results, and file deletions.
- **Portable**: macOS builds are self-contained with bundled FFmpeg dependencies—no global installation required.
- **Fast***: Concurrent scanning handles large libraries with thousands of videos efficiently (*it really depends, but app works as quick as possible).
- **Headless Server with Web UI**: Easy deployment on servers and NAS (Unraid, Synology, Casa/ZimaOS etc.) with Web UI.

## Installation & Setup

### Desktop (macOS & Windows)

1. Download the latest `.dmg` (macOS) or `.exe` (Windows) from the [Releases](https://github.com/vsl86/vdfusion/releases) page.
2. **macOS Users**: Since the app is currently unsigned, you may see a warning. To open:
    - **Right-click** `VDFusion.app` in your Applications folder.
    - Select **Open**.
    - Click **Open** again in the confirmation dialog. (This only needs to be done once).

### Docker (Headless Server)

The headless version is perfect for running on a continuous server. Use the following `docker-compose.yml` to get started:

```yaml
services:
  vdfusion:
    image: vsl86/vdfusion:latest
    ports:
      - "8080:8080"
    volumes:
      - ./storage:/app/storage #persistent storage for DB and logs
      - /your/media/dir/:/media # mount of your media directory
    environment:
      - VDF_DB_PATH=/app/storage/vdf.db
      - VDF_SERVER_ADDR=:8080
      - VDF_LOG_LEVEL=info
    restart: unless-stopped
```

1. Run `docker-compose up -d`.
2. Access the UI at `http://your-server-ip:8080`.

## Key Usage Tips

- **Activity Log**: Click the "Activity Log" button in the status bar to see a history of what the app is doing (e.g., how many files were scanned, which files were deleted).
- **Debug Logging**: If you experience issues or want to see exactly which files are being scanned, enable "Debug Logging" in the scan settings.

## Contributing & Development

Contributions are welcome! If you want to build from source, set up a development environment, or learn about the architecture, please see our [Development Guide](docs/DEVELOPMENT.md).

---
*Created with ❤️ by vsl86*
