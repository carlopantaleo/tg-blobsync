# TG-BlobSync

TG-BlobSync is a conceptual tool to demonstrate the feasibility of using Telegram as a free unlimited blob storage for files. It allows you to sync files between your local machine and Telegram forum topics, which act as buckets (being Telegram forum groups a sort of "storage account", in Azure's terms), in a way similar to `rsync` or `rclone`.

Please bear in mind that Telegram is not intended to be used as a blob storage and I strongly recommend against using it for production purposes.

## Features
- **Bidirectional Sync**: Supports `push` (local to Telegram) and `pull` (Telegram to local) operations.
- **Interactive Browser**: Navigate and explore virtual directories, download individual files using the `browse` command.
- **TUI (Terminal User Interface)**: Modern and interactive UI powered by `Bubble Tea`, featuring real-time progress updates and intuitive selection menus.
- **Efficient Synchronization**: Compare files using MD5 checksums or modification time (`--skip-md5`).
- **Automatic Rate-Limit Recovery**: Operations transparently pause and retry if Telegram returns `FLOOD_WAIT` rate-limit errors.
- **Large File Support**: Files larger than Telegram's 2 GB single-document limit are split into ordered chunks and reassembled during download.
- **Forum Topics as Buckets**: Organizes files within specific Supergroup (Forum) Topics.
- **Smart Handling of Special Files**: Correctly handles 0-byte (empty) files, which are natively rejected by Telegram.
- **Metadata Preservation**: Stores and restores original file modification times and paths.
- **Multi-Session Management**: Manage multiple Telegram accounts and sessions using the `sessions` command.


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

On the first run, the tool will ask for your phone number and the authentication code sent via Telegram. 

### Session Management

TG-BlobSync supports multiple authentication profiles. You can manage them using the `sessions` command:

```bash
tgblobsync sessions
```

This interactive command allows you to:
- **Create New Session**: Perform a new login and add a profile.
- **Select Active Session**: Choose which profile to use for subsequent commands.
- **Delete Session**: Remove an existing profile.

Session files are stored locally in `~/.tg_blobsync/session_<id>.json`.

### Commands

#### Push (Local to Telegram)

Syncs files and directories from a local path to a Telegram Topic.

```bash
tgblobsync push [flags] <local-path> [<group>:<topic>[:subdir]]
```

Example: `tgblobsync push ./my-files "My Supergroup:My Topic"`

#### Pull (Telegram to Local)

Syncs files and directories from a Telegram Topic to a local path.

```bash
tgblobsync pull [flags] [<group>:<topic>[:subdir]] <local-path>
```

Example: `tgblobsync pull "My Supergroup:My Topic:subdir" ./restore-folder`

#### Browse (Interactive Browser)

Explores the virtual directory structure within a Telegram Topic. Allows for interactive navigation and downloading specific files.

```bash
tgblobsync browse [flags] [<group>:<topic>]
```

### Interactive Features

- **Change Confirmation**: Detailed view of planned changes (Upload/Download/Delete) before execution, with a compact single-line layout for easy review.
- **Subdirectory Selection**: Visual selection of existing remote subdirectories with folder icons.
- **Input Waiting**: The tool clearly indicates when an operation is completed or when everything is up to date, waiting for user confirmation before exiting.


### Options

| Flag | Description | Default |
|------|-------------|---------|
| `--workers` | Number of concurrent files to process | 1 |
| `--upload-threads` | Number of parallel threads for a single file upload | 8 |
| `--skip-md5` | Use modification time and size instead of MD5 checksums | false |
| `--no-delete` | Do not delete files that exist only on the target side (`push`/`pull`) | false |
| `--chunk-threshold` | Chunk files larger than this many bytes | 2147483648 |
| `--chunk-size` | Size of each uploaded chunk in bytes | 1073741824 |

> **Note**: For group or topic names containing spaces, wrap them in quotes (e.g., `"My Group:My Topic"`).


## How it works

TG-BlobSync stores file content as documents in Telegram messages. The metadata (relative path, checksum, original modification time) is stored as a JSON object in the message caption.

When syncing:
1. It lists local files and remote messages in the target topic.
2. It compares versions to decide what needs to be uploaded, updated, or deleted.
3. For updates, it uploads the new version and then removes the old message to keep the topic clean.

## Technical Details

- **Empty Files**: Telegram does not allow 0-byte file uploads. TG-BlobSync works around this by uploading a 1-byte dummy file and marking it with an `EMPTY_FILE` flag in the metadata. On `pull`, it restores it as a true 0-byte file.
- **Large Files**: Files larger than `--chunk-threshold` are split into sequential Telegram document messages. Each message is marked with `CHUNK` metadata and an index, while the topic index stores all chunk message IDs. Downloads stream and concatenate chunks in order. The default threshold is 2 GB and the default chunk size is 1 GB; both values are configurable in bytes. Per-chunk progress is shown in the TUI.
- **Session Management**: Securely stores Telegram sessions to avoid repeated logins.

## Disclaimer

This project has been partly developed using Artificial Intelligence and the OpenSpec framework.

## License

MIT
