# logd

logd is a Linux-native log tailing and aggregation utility written in Go. It watches a directory for log files, follows existing `.log` files from the current end of file, starts tailing new `.log` files as they appear, and prints new lines to standard output with the originating file name.

This project is best understood as a combination of two ideas:

- [tiny_tail_f](https://github.com/peter-njuku/tiny_tail_f): a focused single-file tailer that uses Linux `inotify` to follow one file efficiently and handle file rotation-style events.
- [log_aggregator](https://github.com/peter-njuku/log_aggregator): a directory-oriented watcher that discovers and tails multiple `.log` files in one folder.

logd brings those ideas together into a practical, lightweight tool for watching a log directory and streaming activity from multiple sources in real time.

## What it does

When you run logd against a directory, it will:

- scan the directory for existing `*.log` files;
- start following each one from the current end of file;
- watch the directory for newly created `*.log` files and begin tailing them automatically;
- watch each active file for changes and forward new lines to stdout;
- stop cleanly when it receives `SIGINT` or `SIGTERM`.

This makes it useful for:

- debugging multi-service applications;
- watching application logs written to a shared folder;
- observing log rotation and file recreation behavior;
- lightweight local log aggregation without introducing a full logging stack.

## Features

- Linux-only implementation using `inotify` via `golang.org/x/sys/unix`
- Watches a directory for new `.log` files
- Follows existing log files from the current end of file
- Streams new lines to stdout with a `[filename]` prefix
- Tracks file deletion, move-away, and file recreation events
- Uses a shutdown pipe and context cancellation for clean teardown
- Minimal dependencies and straightforward structure

## Architecture overview

The implementation is intentionally small and readable.

### Main components

- `main.go`
  - entry point
  - parses the target directory argument
  - creates the inotify instance
  - watches the target directory for file lifecycle events
  - manages the active set of tailers and shutdown flow

- `tailer.go`
  - defines the per-file tailer state
  - opens each `.log` file
  - seeks to the end so only new content is read
  - uses a buffered reader to stream new lines
  - listens for file change notifications and exits cleanly on cancellation

### Runtime model

At runtime, the program maintains:

- one directory watch for the target folder;
- one file watch per active log file;
- one tailer goroutine per watched file;
- a shared shutdown signal path that stops all active tailers.

## How it works

1. The program accepts one argument: the directory to watch.
2. It scans the directory for existing `*.log` files and starts a tailer for each one.
3. It registers an `inotify` watch on the directory for create/delete/move events.
4. When a new `.log` file is created, it opens it, seeks to the end, and starts streaming new data.
5. When an active file changes, the corresponding tailer reads the new data and writes it to stdout.
6. On shutdown, the program cancels all active tailers and waits for them to exit.

## Requirements

- Linux operating system
- Go 1.26.3 or newer (as declared in `go.mod`)
- A filesystem that supports inotify events

## Installation

Clone the repository:

```bash
git clone https://github.com/peter-njuku/logd.git
cd logd
go build -o logd .
```

You can also run it directly without building:

```bash
go run . /path/to/log/dir
```

## Usage

# logd

`logd` is a small Linux-native log tailing and aggregation utility written in Go. It watches a directory for `*.log` files, follows each file from the current end, tails newly created `*.log` files, and forwards new log lines as structured JSON records to one of several output targets (stdout, UDP, or WebSocket).

The implementation combines efficient event-driven tailing using `inotify` with a simple multi-file manager to follow many files concurrently.

**Quick summary**

- Watches a directory for `*.log` files and tails them from the current end-of-file
- Emits newline-delimited JSON records (one JSON object per log line)
- Supports writing to `stdout`, sending JSON over UDP (`-udp`), or broadcasting to WebSocket clients (`-ws`)
- Clean shutdown on `SIGINT`/`SIGTERM`

## What it does

When you run `logd` against a directory it will:

- scan the directory for existing `*.log` files and start tailing each one from EOF
- register an `inotify` watch on the directory to discover new or removed files
- register an `inotify` watch per active file to detect modifications and rotation-like events
- emit each new log line as a JSON object with `file`, `line`, and `timestamp` fields
- optionally forward the JSON lines to a UDP endpoint or broadcast them to connected WebSocket clients

## Command-line flags

- `-udp <host:port>` : send each JSON log line as a single UDP packet to the given address (e.g. `localhost:514`)
- `-ws <addr>` : listen on the specified HTTP address and serve WebSocket clients at the `/ws` path (e.g. `:8080`)
- `-verbose` : print debug information to stderr
- `-help` : show usage

## Output format

Each emitted line is a JSON object followed by a single newline. Example:

```json
{"file":"app.log","line":"user logged in","timestamp":"2026-07-02T12:34:56.789012345Z"}
```

- `file`: the basename used to identify the file the line came from
- `line`: the log text (the trailing newline is removed)
- `timestamp`: RFC3339Nano UTC timestamp added at emission time

When running without `-udp` or `-ws` the JSON lines are written to `stdout`. With `-udp` they are sent as UDP packets to the configured address. With `-ws` the server accepts WebSocket clients on `/ws` and sends each JSON message to connected clients.

## Examples

Run against a directory and print JSON to stdout:

```bash
./logd /path/to/log/dir
```

Send JSON lines over UDP:

```bash
./logd -udp localhost:514 /path/to/log/dir
```

Start an HTTP server and broadcast to WebSocket clients on `/ws`:

```bash
./logd -ws :8080 /path/to/log/dir
# then connect a browser or websocket client to ws://localhost:8080/ws
```

## Files of interest

- [main.go](main.go) — program entrypoint, flag parsing, `inotify` directory watch, tailer lifecycle management, shutdown handling
- [tailer.go](tailer.go) — per-file tailer: opens file, seeks to EOF, reads new lines and marshals JSON records
- [websocket.go](websocket.go) — optional WebSocket broadcaster and `/ws` handler

## Requirements

- Linux with `inotify` support
- Go 1.26.3 or newer (see `go.mod`)

## Build

```bash
git clone https://github.com/peter-njuku/logd.git
cd logd
go build -o logd .
```

Run without building:

```bash
go run . /path/to/log/dir
```

## Behavior notes and caveats

- The program begins tailing from EOF, so it will not print existing historical content.
- Lines are assumed to be newline-terminated; partial lines will be delivered once a newline is observed.
- The tailer writes JSON to the configured sink; if a UDP sink is used, packets may be dropped or truncated by the network.
- The WebSocket server exposes `/ws` and sends raw JSON messages; clients should expect newline-delimited JSON per message.
- The implementation is intentionally small and focused; it does not attempt to be a full-featured log transport system.

## Developer notes

Potential improvements:

- add optional filtering or include/exclude patterns
- support recursive directory watching
- add backpressure or batching for network sinks
- add tests for rotation/truncation scenarios

## License

No license has been declared. Please add a license file if you intend to reuse or redistribute this code.
