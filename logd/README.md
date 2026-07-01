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

Run the binary and point it at a directory that contains log files:

```bash
./logd /path/to/log/dir
```

Example:

```bash
mkdir -p /tmp/example-logs
printf 'first line\n' > /tmp/example-logs/app.log
./logd /tmp/example-logs
```

Then append more data:

```bash
printf 'second line\n' >> /tmp/example-logs/app.log
```

You should see output similar to:

```text
[app.log] - second line
```

## Output format

Each new line is emitted to stdout in the following form:

```text
[<filename>] - <line content>
```

This makes it easy to spot which source file produced the output.

## Behavior notes

- The program starts at the current end of each file, so it does not print historical contents.
- It is line-oriented and assumes log entries are newline-delimited.
- It is designed for local, lightweight use rather than high-scale distributed logging.
- It depends on Linux kernel event notifications and is not portable to non-Linux systems.

## Developer notes

### Code organization

The project is intentionally small, which makes it easy to extend.

Potential extension points include:

- replacing stdout with a callback, queue, or sink interface;
- adding configuration flags for output destination, verbosity, or follow mode;
- supporting recursive directory watching;
- adding structured parsing for JSON or other log formats;
- improving handling for truncation, rotation, and inode changes;
- adding tests for create/delete/rename/recreate flows.

### Current limitations

- Linux-specific
- No configuration file or flags yet
- No built-in filtering, parsing, or routing
- Output is plain text only

## Why this project exists

The repository was created as a practical synthesis of the tailing and aggregation concepts explored in the two referenced projects:

- `tiny_tail_f` contributed the efficient, event-driven tailing approach for a single file.
- `log_aggregator` contributed the directory-level idea of discovering and following multiple logs in one place.

This repository combines both approaches into a single tool that can follow a folder of log files without a polling loop.

## Contributing

Contributions are welcome. If you want to improve the tool, consider:

- adding tests;
- improving cross-file lifecycle handling;
- introducing configurable output targets;
- documenting behavior around log rotation and truncation.

## License

No license has been declared in the repository yet. If you plan to reuse or redistribute this code, confirm the licensing terms before doing so.
