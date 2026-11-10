# ffmpeg-stripped

A Go library that embeds stripped-down FFmpeg 7.1.2 executables for local video
processing. Only the target platform’s gzip-compressed binary is embedded and
unpacked on demand. No CGO, separate FFmpeg installation, or runtime downloads.

## Install

```sh
go get github.com/Warp-net/ffmpeg-stripped
```

## Usage

```go
import ffmpeg "github.com/Warp-net/ffmpeg-stripped"

binary, err := ffmpeg.GetFFmpeg()
if err != nil {
    return err
}
// Use ffmpeg.exe on Windows. Choose a writable location for the executable.
if err := os.WriteFile("ffmpeg", binary, 0o700); err != nil {
    return err
}
cmd := exec.Command("./ffmpeg", "-version")
cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
return cmd.Run()
```

The example also uses `os` and `os/exec`. Each `GetFFmpeg()` call returns a fresh
`[]byte`. Linux, macOS, and Windows support both `amd64` and `arm64`; Android,
iOS, and other targets return `ffmpeg.ErrUnsupported`.

## Included features

- Local MP4/MOV input with H.264 video and PNG images.
- Frame-rate conversion, image overlays, and MP4/H.264 output via x264.
- Raw decoded frames (`-f rawvideo`), for example small grayscale frames for analysis.
- Copying MP4-compatible audio without decoding or re-encoding.

Other video decoders, networking, capture devices, `ffprobe`, and `ffplay` are
excluded. x264 supports 8-bit YUV 4:2:0 only. Assembly optimizations are disabled,
which can reduce performance.

Linux binaries are fully static (musl). Windows requires system DLLs, including
the Universal CRT. macOS requires version 11 or later; binaries are not notarized.
No separate FFmpeg, x264, or zlib shared libraries are needed.

## Frame replacement example

For full-frame replacement the overlay PNG must be opaque and match the video
dimensions; a PNG with an alpha channel is blended over the frame instead. This
replaces frames 24, 49, and so on in the 25 FPS output, while copying compatible audio:

```sh
./ffmpeg -i input.mp4 -loop 1 -framerate 25 -i frame.png \
  -filter_complex "[0:v]fps=25,setpts=PTS-STARTPTS[v];[v][1:v]overlay=0:0:shortest=1:enable='eq(mod(n,25),24)'[out]" \
  -map '[out]' -map '0:a?' -r 25 \
  -c:v libx264 -crf 18 -pix_fmt yuv420p -c:a copy output.mp4
```

Keep the explicit output `-r 25`: without it, the original test produced a final
MP4 frame with zero duration that was not returned during decoding.

## How the binaries were built

Built on November 6, 2026, and rebuilt on November 10, 2026, with the `rawvideo`
encoder and muxer added, on Linux x86-64 using Zig as a cross-compiler.
This configuration targets the workflow above; it is not a general-purpose
FFmpeg build or a proven minimum-size build.

| Component | Version / source |
|---|---|
| FFmpeg | 7.1.2, tag `n7.1.2` |
| x264 | `mirror/x264` master snapshot downloaded on the build date |
| zlib | 1.3.1 |
| Zig | 0.13.0, Linux x86-64 distribution |
| Clang bundled with Zig | 18.1.6 |

The exact FFmpeg, x264, and zlib source trees and original build scripts are in
[sources/ffmpeg-minimal-sources.tar.xz](sources/ffmpeg-minimal-sources.tar.xz).
The x264 snapshot is preserved there: downloading `master` again may give
different code. Original download checksums are in
[sources/UPSTREAM-SHA256SUMS](sources/UPSTREAM-SHA256SUMS). The toolchain library
sources, including musl and MinGW-w64, are also [provided](sources/zig-0.13.0-library-sources.tar.xz).

### Targets and executable sizes

Sizes are uncompressed bytes, excluding source archives and documentation.
`macos-*` names below are the original build target names; Go uses `darwin`.

| Build target | Zig target | Format | Bytes |
|---|---|---|---:|
| `linux-x86_64` | `x86_64-linux-musl` | ELF x86-64 | 3,296,096 |
| `linux-arm64` | `aarch64-linux-musl` | ELF AArch64 | 3,160,504 |
| `windows-x86_64` | `x86_64-windows-gnu` | PE32+ x86-64 | 3,066,368 |
| `windows-arm64` | `aarch64-windows-gnu` | PE32+ AArch64 | 2,719,744 |
| `macos-x86_64` | `x86_64-macos.11.0` | Mach-O x86-64 | 3,091,975 |
| `macos-arm64` | `aarch64-macos.11.0` | Mach-O ARM64 | 3,044,744 |

### Dependencies and configuration

Each target gets separate build and installation directories. Wrappers invoke
Zig's `cc`, `ar`, and `ranlib`; `cc` receives the target from the table.
`PKG_CONFIG_PATH` and `PKG_CONFIG_LIBDIR` point only to that target's dependencies.

First, zlib is built as a static library with `CFLAGS=-Os`. Then x264 is built
with the target-specific `--host` and `--prefix`, these options, and the linker
flags described below:

```sh
--enable-static --disable-cli --disable-opencl --disable-asm
--bit-depth=8 --chroma-format=420 --extra-cflags=-Os
```

No standalone x264 CLI is built. FFmpeg is configured with:

```sh
--enable-cross-compile
--disable-everything --disable-autodetect --disable-network
--disable-doc --disable-debug --disable-ffplay --disable-ffprobe
--disable-avdevice --disable-shared --enable-static
--enable-small --disable-asm --disable-stripping
--enable-gpl --enable-libx264 --enable-zlib
--enable-protocol=file
--enable-demuxer=mov,image2,png_pipe --enable-muxer=mp4,rawvideo
--enable-decoder=h264,png --enable-encoder=libx264,rawvideo
--enable-parser=h264,png
--enable-filter=buffer,buffersink,fps,setpts,overlay,scale,format,null
--pkg-config-flags=--static
```

The script also supplies `--cc`, `--ar`, `--ranlib`, `--target-os`, `--arch`,
`--extra-cflags=-I<target-prefix>/include`, and
`--extra-ldflags=-L<target-prefix>/lib <linker-flags>`. It runs `make -j4 ffmpeg`
for Linux/macOS and `make -j4 ffmpeg.exe` for Windows. FFmpeg automatically
enables some dependencies, so the filter list above is not the complete list
in the executable. The original per-target `config.mak` files are preserved in
[sources/build-configs.tar.xz](sources/build-configs.tar.xz); the executable also
reports its configure arguments with `-version`.

### Linking, size reductions, and source changes

- Linux uses musl with `-static -s`: executables are fully static and stripped.
- Windows uses `-static -s`, retaining imports of system DLLs including the
  Universal CRT, `KERNEL32.dll`, `SHELL32.dll`, and `bcrypt.dll`.
- macOS uses `-Wl,-S` to remove debugging information. The only dynamic dependency
  found was `/usr/lib/libSystem.B.dylib`; the deployment target is macOS 11+.
  The executables are not notarized.

`--disable-stripping` skips FFmpeg's separate final `strip` invocation; the
linker flags above still remove symbols/debugging information. Most size
savings come from disabled codecs, formats, networking, devices, documentation,
and extra programs, plus `--enable-small`. Assembly optimizations are disabled
in both FFmpeg and x264, which can substantially reduce speed. Performance was
not benchmarked.

Two changes to FFmpeg's `configure` accommodate Zig's linker:

1. Remove Darwin's `-Wl,-dynamic,-search_paths_first`; Zig rejects `-dynamic`
   and already uses dynamic system linking by default.
2. Remove Windows x86-64's `-Wl,--pic-executable,-e,mainCRTStartup` binutils
   workaround; Zig rejects `--pic-executable`. ASLR options remain enabled.

The [patch](sources/ffmpeg-configure.patch) is already applied in the supplied
sources. x264 and zlib were not modified. For macOS, zlib's build receives
`AR=<target-ar> ARFLAGS=rc` explicitly to avoid requiring Apple's `libtool`
on the Linux build host.

### Rebuild and update embedded binaries

On Linux x86-64, install Bash, make, pkg-config, Python 3, and tar/xz. Starting
at the repository root, verify and extract the supplied archives into a new
workspace:

```sh
(cd sources && sha256sum -c SHA256SUMS)
mkdir rebuild
tar -xJf sources/ffmpeg-minimal-sources.tar.xz -C rebuild
tar -xJf sources/zig-0.13.0-library-sources.tar.xz -C rebuild
cd rebuild
curl -fL https://ziglang.org/download/0.13.0/zig-linux-x86_64-0.13.0.tar.xz -o zig.tar.xz
echo 'd45312e61ebcc48032b77bc4cf7fd6915c11fa16e4aad116b66c9468211230ea  zig.tar.xz' | sha256sum -c -
tar -xJf zig.tar.xz -C work
```

Run the original build commands sequentially; each uses four make jobs:

```sh
bash work/build-minimal.sh linux-x86_64 x86_64-linux-musl x86_64-linux-musl linux x86_64
bash work/build-minimal.sh linux-arm64 aarch64-linux-musl aarch64-linux-musl linux aarch64
bash work/build-minimal.sh windows-x86_64 x86_64-windows-gnu x86_64-w64-mingw32 mingw32 x86_64
bash work/build-minimal.sh windows-arm64 aarch64-windows-gnu aarch64-w64-mingw32 mingw32 aarch64
bash work/build-minimal.sh macos-x86_64 x86_64-macos.11.0 x86_64-apple-darwin darwin x86_64
bash work/build-minimal.sh macos-arm64 aarch64-macos.11.0 aarch64-apple-darwin darwin aarch64
```

Outputs are under `rebuild/outputs/ffmpeg-minimal/<build-target>/`. From the
repository root, replace the matching archive in `bin/` using `gzip -n -9 -c`,
for example:

```sh
gzip -n -9 -c rebuild/outputs/ffmpeg-minimal/linux-x86_64/ffmpeg > bin/ffmpeg-linux-amd64.gz
```

Other filenames follow `ffmpeg-<GOOS>-<GOARCH>[.exe].gz`, with `darwin` for macOS
and `.exe` for Windows. Update the source snapshots, configuration records,
checksums, and notices together with any new binaries. Bit-for-bit rebuilds
across different host environments and workspace paths have not been verified.

## Verification of the original binaries

| Target | Original checks |
|---|---|
| Linux x86-64 | Execution, PNG-to-MP4/H.264 encoding, decoding, frame replacement, AAC preservation |
| Windows x86-64 | Execution and frame replacement under Wine, output video verification |
| Linux ARM64 | ELF format, architecture, static linking; not executed |
| Windows ARM64 | PE format, architecture, DLL imports; not executed |
| macOS x86-64 and ARM64 | Mach-O format, architecture, dynamic dependencies; not executed |

The November 10 rebuild was executed on Linux x86-64 only: it decoded a 600-frame
720p H.264 clip to 160×90 grayscale `rawvideo` and encoded one frame with an
alpha PNG overlay through x264. The other five targets were cross-built from the
same scripts and not executed.

The frame-replacement test decoded all 50 frames of a two-second black video
at 25 FPS with a white PNG overlay: zero-based frames 24 and 49 were white,
and all others were black. This passed on Linux x86-64 and Windows x86-64
under Wine. Wine testing does not establish native Windows compatibility.

On Linux, every AAC packet was compared before and after processing and matched
byte-for-byte. The concatenated packet data had SHA-256:

```text
73fc5e586b56dfb9f275874d6cf43ca1d00054761e822fca557878d51d4f1a75
```

The source archive includes `work/verify-frames.c` and `work/dump-audio.c`.
[sources/BINARY-SHA256SUMS](sources/BINARY-SHA256SUMS) records the uncompressed
executable checksums; [sources/SHA256SUMS](sources/SHA256SUMS) covers the supplied
source/configuration archives. Go package tests check unpacking and execution
on the host; cross-builds were also checked for all six supported targets and
six unsupported targets, where no executable is embedded.

## License

The Go wrapper and bundled FFmpeg/x264 executables are distributed under
GPL-2.0-or-later; see [LICENSE](LICENSE). Third-party license texts and notices
are in [LICENSES](LICENSES/README.md). Corresponding sources, build instructions,
and checksums are in [sources](sources/README.md).

When redistributing the embedded executables, preserve the applicable licenses
and notices and provide corresponding source code as required by the GPL.
