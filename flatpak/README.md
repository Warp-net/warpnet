# Flatpak / Flathub

Files for packaging Warpnet as a flatpak and publishing it on Flathub.

- `site.warpnet.Warpnet.yml` — flatpak-builder manifest
- `site.warpnet.Warpnet.desktop` — desktop entry
- `site.warpnet.Warpnet.metainfo.xml` — AppStream metadata

## CI

`.github/workflows/flatpak.yml` builds the flatpak bundle and runs the Flathub
linter on every push to the `flatpak` branch (same trigger model as the snap
pipeline). The resulting `warpnet.flatpak` bundle is uploaded as a workflow
artifact.

Local build:

```sh
flatpak-builder --user --install-deps-from=flathub --force-clean build-dir flatpak/site.warpnet.Warpnet.yml
```

## Publishing to Flathub

Unlike the Snap Store, Flathub does not accept package uploads from CI.
The flow is:

1. Tag a release and replace the `dir` source in the manifest with a pinned
   `git` source (url + tag + commit), see the comment in the manifest.
2. Open a pull request against https://github.com/flathub/flathub
   (branch off `new-pr`) adding `site.warpnet.Warpnet.yml`, and pass review.
   See https://docs.flathub.org/docs/for-app-authors/submission
3. After acceptance, Flathub creates the `flathub/site.warpnet.Warpnet`
   repository and builds/publishes the app on its own infrastructure.
   Updates are commits to that repository bumping the tag/commit.
4. Verify ownership of warpnet.site to mark the app as verified:
   https://docs.flathub.org/docs/for-app-authors/verification
