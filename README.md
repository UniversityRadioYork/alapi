ALAPI - Audio Levels API
===

This is a simple little daemon that runs on a system, listens to incoming audio,
and exposes a HTTP endpoint that tells people how loud it is. That's it.

## Building

You'll need [libsoundio](http://libsound.io/) headers on your system - the exact approach
depends on your OS:

* Debian/Ubuntu: `sudo apt install libsoundio-dev`
* Arch: install `libsoundio` with your favourite AUR helper
* macOS: `brew install libsoundio`
* Windows: download the [unofficial binaries](https://github.com/cameronmaske/libsoundio-binaries) or [build it yourself](https://github.com/andrewrk/libsoundio/wiki/Compiling-for-Windows)

Apart from that you'll need Go 1.17 or later and a C compiler. If you're on Linux
install gcc or clang, on macOS install the Xcode Command Line Tools, or on Windows
install MinGW-64.

Then, clone this repository, and run:

```sh
$ go mod download
$ go build -o ./alapi ./cmd/alapi
```

And run `./alapi`.

## Running

Create a `config.json` file, following the example of `config.json.example` and `pkg/config/config.go`.
Here's what the fields mean:

* `Backend`: the audio backend to use (if you're not sure what to use, alapi will print all known backends on startup)
* `Devices`: a mapping of keys to device names - the keys can be anything, they're what will be referenced by the API
* `BufferLength`: how long to listen for
* `Bind`: the hostname to bind for - for all IPs, use `"0.0.0.0"`
* `Port`: the port to listen on

Then, run `./alapi -cfg=./config.json`.
