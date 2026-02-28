## Prerequisites

- **Go 1.20+**
- **Node.js 18+ & npm** (for the Vue frontend module)
- **FFmpeg & FFprobe** 
    - macOS: `brew install ffmpeg`
    - Make sure these binaries are available in your system `PATH`.

## How to Run in Development Mode

`wails dev` starts a development server with hot-reloading for both Go and the Vue frontend.

If you are using Homebrew on macOS (Apple Silicon or Intel), ensure that the Homebrew `bin` folder is correctly in your `PATH` before running Wails commands, as `npm` and other tools reside there.

```bash
# Export the Homebrew path if using macOS
export PATH=$PATH:/opt/homebrew/bin

# Start the Wails Development Server
wails dev
```

This command will open the desktop application with Developer Tools available, and any changes you make in `frontend/src` will automatically trigger a reload.

## Building the Application

To build a standalone production release for your platform:

```bash
export PATH=$PATH:/opt/homebrew/bin
wails build
```

This will run the full production build sequence. On macOS, the result will be a `.app` bundle located in `build/bin/vdfusion.app`.

## Testing with a Fake Database

For UI testing and performance benchmarking, you can generate a large database populated with fake file records and duplicates without needing terabytes of actual video files.

1. **Generate the fake database** using the `fakegen` utility:
   ```bash
   # Generates a DB with 100,000 unique files and 500 duplicate groups
   # (Note: the path-prefix is what the UI will group videos by)
   go run cmd/fakegen/main.go -entries 100000 -duplicate-groups 500 -output fake_vdf.db -path-prefix /fake_files
   ```
   *Available flags: `-entries`, `-seed`, `-thumbnails`, `-min-duration`, `-max-duration`, `-duplicate-groups`, `-duplicate-group-size`, `-output`, `-path-prefix`.*

2. **Run VDFusion** using the generated database. The fakegen tool will automatically add the `path-prefix` to your `settings.json` so you do not have to configure it.
   ```bash
   # Desktop app (Wails dev mode)
   VDF_DB_PATH=fake_vdf.db wails dev
   ```

3. **View Results**: The app loads empty because results are not persisted between runs. To see the generated duplicates, simply click the **"Start Scan"** button in the UI. The walker will instantly pass the missing fake directory, but the comparison engine will load all fake records from the DB and find your duplicates!

## Project Structure

- `app.go`: The main backend logic and Wails bindings. Exposes functions to `Wails.js`.
- `main.go`: Application entry point and dependencies initialization.
- `internal/`: Migrated Go packages (`db`, `engine`, `media`, `phash`, `config`).
- `cmd/fakegen/`: Utility for generating fake databases for UI/performance testing.
- `frontend/src`: Vue 3 application components (`App.vue` manages tabs, `ScanSettings.vue`, `ProgressBar.vue`, `ResultsGrid.vue`).
- `frontend/wailsjs`: Auto-generated bindings from the Go backend to Javascript. Do not edit manually.
