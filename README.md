# TG-BlobSync

TG-BlobSync is a powerful CLI tool written in go, designed to synchronize local directories with Telegram Forum Topics. It uses Telegram as a blob storage, allowing you to back up and sync files efficiently.

This is a conceptual tool to demonstrate the feasibility of using Telegram as a free unlimited blob storage for files. Please bear in mind that Telegram is not intended to be used as a blob storage and I strongly recommend against using it for production purposes.

## Features

- **Bidirectional Sync**: Supports `push` (local to Telegram) and `pull` (Telegram to local) operations.
- **Interactive Browser**: Navigate and explore virtual directories and files in a topic using the `list` command.
- **Efficient Synchronization**: Compare files using MD5 checksums or modification time (`--skip-md5`).
- **High Performance**: Multithreaded file processing and parallelized chunk uploads for large files.
- **Telegram Forum Support**: Organizes files within specific Supergroup Topics.
- **Smart Handling of Special Files**: Correctly handles 0-byte (empty) files, which are natively rejected by Telegram.
- **Metadata Preservation**: Stores and restores original file modification times and paths.
- **Non-Interactive Mode**: Fully scriptable with the `--non-interactive` flag.
- **Beautiful UI**: Interactive progress bars and selection menus using `mpb` and `promptui`.

## Installation

### Prerequisites

- Go 1.25.5 or later.
- Telegram API credentials (`AppID` and `AppHash`) from [my.telegram.org](https://my.telegram.org).

### Build

```bash
go build -ldflags "-X main.AppID=YOUR_APP_ID -X main.AppHash=YOUR_APP_HASH" ./cmd/tgblobsync
```

Alternatively, you can set `APP_ID` and `APP_HASH` as environment variables.

## Usage

### Authentication

On the first run, the tool will ask for your phone number and the authentication code sent via Telegram. A session file will be stored locally (typically in `~/.tg_blobsync/session.json`) for future use.

### Commands

#### Push (Local to Telegram)

Uploads files from a local directory to a Telegram Topic interactively.

```bash
tgblobsync push --dir ./my-files
```

#### Pull (Telegram to Local)

Downloads files from a Telegram Topic to a local directory interactively.

```bash
tgblobsync pull --dir ./restore-folder
```

#### List (Interactive Browser)

Explores the virtual directory structure within a Telegram Topic.

```bash
tgblobsync list [ --group-id <ID> [ --topic-id <ID> ] ]
```

### Options

| Flag | Description | Default |
|------|-------------|---------|
| `--dir` | Path to the directory to sync (Required for push/pull) | - |
| `--sub-dir` | Synchronize only a specific subdirectory within the topic | - |
| `--group-id` | ID of the Supergroup | Interactive selection |
| `--topic-id` | ID of the Topic (TopID) | Interactive selection |
| `--workers` | Number of concurrent files to process | 4 |
| `--upload-threads` | Number of parallel threads for a single file upload | 8 |
| `--skip-md5` | Use modification time and size instead of MD5 checksums | false |
| `--non-interactive` | Disable interactive UI and progress bars | false |

## How it works

TG-BlobSync stores file content as documents in Telegram messages. The metadata (relative path, checksum, original modification time) is stored as a JSON object in the message caption.

When syncing:
1. It lists local files and remote messages in the target topic.
2. It compares versions to decide what needs to be uploaded, updated, or deleted.
3. For updates, it uploads the new version and then removes the old message to keep the topic clean.

## Technical Details

- **Empty Files**: Telegram does not allow 0-byte file uploads. TG-BlobSync works around this by uploading a 1-byte dummy file and marking it with an `EMPTY_FILE` flag in the metadata. On `pull`, it restores it as a true 0-byte file.
- **Large Files**: Files are uploaded in chunks. The tool automatically optimizes chunk size and uses multiple connections to saturate available bandwidth. Please note that files bigger than 2 GB are not supported by Telegram (4 GB for premium users).
- **Session Management**: Securely stores Telegram sessions to avoid repeated logins.

## License

MIT
