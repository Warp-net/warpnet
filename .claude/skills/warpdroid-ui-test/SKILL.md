---
name: warpdroid-ui-test
description: Use this skill whenever a warpdroid (Android client) change or bug needs to be verified by actually driving the app's visual UI — any task phrased as "test the warpdroid UI", "click through the screens", "does the button/compose/chat actually work on the device", "прокликать интерфейс", "проверь экран X на андроиде", "smoke-test warpdroid on an emulator", "take screenshots of the app", or when you've changed an Activity/Compose screen/adapter and want on-device confirmation rather than just a build. This skill MANDATES bringing up an Android emulator, installing the app, pairing it with a real Warpnet node, and driving it black-box via `adb`/`uiautomator` — that is the required verification vehicle. Do NOT use this skill to add a new protocol route (use `warpnet-add-handler`), to verify a node handler over `/ws` (use `warpnet-testnet-verify`), or to diagnose a cross-layer wire bug whose symptom you already have (use `warpnet-debug-stack`). Use it to confirm the Android UI itself behaves on a running device.
---

# Testing the warpdroid visual UI on an emulator via adb

warpdroid is a **Tusky fork** (native Android, Kotlin, appId `site.warpnet.warpdroid`).
It is a *thin client*: an embedded Go node (gomobile `libgojni.so`, via
`warpdroid/node` → `warpnet-transport`) does all the networking, and the app **must be
paired with a running Warpnet fat node** before any real screen works.

**Why black-box adb and not instrumented tests:** there are **no androidTest sources** —
the androidTest variant is deliberately disabled in `warpdroid/app/build.gradle`, and the
Compose screens (chats, timeline `TweetCard`) carry **no `testTag`s**. So the only way to
exercise the visual UI is to run the APK on a device/emulator and drive it with
`uiautomator dump` (element bounds) + `adb shell input tap/text` + `screencap`.

**This skill's rule: a warpdroid UI change is not "tested" until the app has been
installed on a running emulator, paired with a live node, and the affected screen has
been driven and screenshotted showing the expected result.** A successful gradle build is
never sufficient.

## Two hard prerequisites (do these first, in order)

### Prereq A — the x86_64 native library

The standard emulator is x86_64, but `warpdroid/node/build-native.sh` builds the gomobile
aar for **arm64 only** (`-target=android/arm64`). A stock debug APK therefore crashes on
an x86_64 emulator at pairing time:

```
java.lang.UnsatisfiedLinkError: dlopen failed: library "libgojni.so" not found
    at go.Seq.<clinit> → node.Node.<clinit> → WarpnetClient.initialise
```

Fix it once. Preferred: rebuild the aar with both ABIs, then a normal gradle build. Fast
path (no full gradle build): patch the existing debug APK with the x86_64 `.so` and
re-sign.

```bash
cd warpdroid/node
export ANDROID_HOME=~/Android/Sdk ANDROID_NDK_HOME=~/Android/Sdk/ndk/<ver> PATH=$PATH:~/go/bin
GOFLAGS=-mod=mod gomobile bind -ldflags="-checklinkname=0 -s -w" -trimpath -tags mobile \
    -androidapi 24 -target=android/arm64,android/amd64 -o warpnet_multi.aar .
# -> warpnet_multi.aar now contains jni/arm64-v8a + jni/x86_64 libgojni.so
```

Fast-path APK patch (avoids a multi-minute gradle build):

```bash
BT=~/Android/Sdk/build-tools/<ver>
SRC=warpdroid/app/build/outputs/apk/debug/Warpdroid_*_debug_x86_64.apk
mkdir -p /tmp/apkwork/lib/x86_64
unzip -o -j warpdroid/node/warpnet_multi.aar jni/x86_64/libgojni.so -d /tmp/apkwork/lib/x86_64
cp "$SRC" /tmp/warp.apk
( cd /tmp/apkwork && zip -0 -X /tmp/warp.apk lib/x86_64/libgojni.so )
zip -d /tmp/warp.apk "META-INF/*.RSA" "META-INF/*.SF" "META-INF/*.MF"   # drop old sig
$BT/zipalign -p -f 4 /tmp/warp.apk /tmp/warp_aligned.apk
$BT/apksigner sign --ks ~/.config/.android/debug.keystore --ks-pass pass:android \
    --ks-key-alias androiddebugkey --key-pass pass:android /tmp/warp_aligned.apk
```

> **Cleanup:** `warpnet_multi.aar` / `*-sources.jar` are build artifacts — delete them and
> `git checkout -- go.mod go.sum` afterward (the gomobile build touches them). Never commit
> a modified `warpnet.aar`. Confirm `git status` is clean before finishing.

### Prereq B — the pairing payload (`AuthNodeInfo`)

MainActivity gates everything: on launch it loads `PairedNodeStore`; if empty it jumps to
`PairingActivity` and **no other screen is reachable**. Pairing needs an `AuthNodeInfo`
JSON, which the fat node's desktop UI shows as a QR ("Pair your phone" / "Sign in by
QR-code"). The emulator has no camera, so use **manual entry** instead — the field
accepts both plain JSON and the raw Base45 QR blob (`AuthNodeInfoValidator` +
`QrPayloadCodec`).

Get the JSON either way:

- **From a screenshot of the QR** — decode Base45→gzip→JSON (the frontend encodes it in
  `frontend/src/lib/qr-payload.js`: gzip level 9 then Base45/RFC 9285):

  ```bash
  pip3 install --user --break-system-packages opencv-python-headless numpy   # if missing
  python3 - <<'PY'
  import cv2, gzip, json
  data,_,_ = cv2.QRCodeDetector().detectAndDecode(cv2.imread('/path/to/qr.png'))
  ALPH="0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"; T={c:i for i,c in enumerate(ALPH)}
  def b45(s):
      out=bytearray(); full=len(s)//3
      for i in range(full):
          v=T[s[i*3]]+T[s[i*3+1]]*45+T[s[i*3+2]]*2025; out+=bytes([v>>8,v&0xFF])
      if len(s)%3==2:
          v=T[s[full*3]]+T[s[full*3+1]]*45; out+=bytes([v&0xFF])
      return bytes(out)
  open('/tmp/authnodeinfo.json','w').write(gzip.decompress(b45(data.strip())).decode())
  print(open('/tmp/authnodeinfo.json').read())
  PY
  ```

- **Or ask the user** to paste the pairing JSON from the desktop node.

A valid payload has `token`, `psk`, `node_id`, `user_id`, non-empty `addresses`, and
`network` (e.g. `testnet`). The node behind those `addresses` must be running and
reachable from the emulator (public IP, or `10.0.2.2` for a node on the emulator host).

## The mandatory loop

```
1. native   ensure the APK has an x86_64 libgojni.so   (Prereq A)
2. payload  obtain AuthNodeInfo JSON                    (Prereq B)
3. avd      create + boot an x86_64 AVD
4. install  adb install the patched APK
5. pair     deny camera → manual JSON entry → Pair → Connect
6. drive    tap/type/screenshot the screen(s) you changed
7. assert   read the screenshot / uiautomator dump for the expected result
8. teardown (optional) leave the emulator up for reuse; clean repo artifacts
```

## Step 3 — create & boot the emulator

```bash
SDK=~/Android/Sdk
$SDK/cmdline-tools/latest/bin/avdmanager create avd -n warp35 \
    -k "system-images;android-35;default;x86_64" -d pixel_6 --force
# avdmanager may write the AVD under ~/.config/.android/avd — point the emulator at it:
ANDROID_AVD_HOME=~/.config/.android/avd $SDK/emulator/emulator -avd warp35 \
    -no-snapshot -no-audio -gpu swiftshader_indirect -no-boot-anim &
$SDK/platform-tools/adb wait-for-device
# wait for boot:
until [ "$($SDK/platform-tools/adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ]; do sleep 3; done
```

> **AVD-home gotcha:** `avdmanager` often creates the AVD in `~/.config/.android/avd/`
> while the emulator by default searches `~/.android/avd`, giving
> `Unknown AVD name`. Always pass `ANDROID_AVD_HOME` pointing at the real folder.

## Step 4 — install

```bash
ADB=~/Android/Sdk/platform-tools/adb
$ADB install -r /tmp/warp_aligned.apk        # or uninstall first if signature differs:
# $ADB uninstall site.warpnet.warpdroid; $ADB install /tmp/warp_aligned.apk
$ADB shell am start -n site.warpnet.warpdroid/.MainActivity
```

## The UI driver helper

Drop this once; it finds an element's bounds via `uiautomator dump` and taps the centre.
Use it for every interaction so you never hard-code coordinates you can't recompute.

```bash
cat > /tmp/ui.sh <<'EOF'
#!/usr/bin/env bash
ADB=~/Android/Sdk/platform-tools/adb
dump() { $ADB shell uiautomator dump /sdcard/ui.xml >/dev/null 2>&1; $ADB shell cat /sdcard/ui.xml; }
shot() { $ADB exec-out screencap -p > "${1:-/tmp/shot.png}"; echo "saved ${1:-/tmp/shot.png}"; }
# tap centre of first node matching an attribute pattern, e.g.:
#   tap_attr 'text="Send"'   tap_attr 'resource-id="site.warpnet.warpdroid:id/composeButton"'
#   tap_attr 'content-desc="New message"'   tap_attr 'class="android.widget.EditText"'
tap_attr() {
  dump | tr '<' '\n' | grep -- "$1" \
    | grep -oE 'bounds="\[[0-9]+,[0-9]+\]\[[0-9]+,[0-9]+\]"' | head -1 \
    | sed -E 's/bounds="\[([0-9]+),([0-9]+)\]\[([0-9]+),([0-9]+)\]"/\1 \2 \3 \4/' \
    | { read x1 y1 x2 y2; [ -n "$x1" ] && { $ADB shell input tap $(((x1+x2)/2)) $(((y1+y2)/2)); \
        echo "tap $(((x1+x2)/2)) $(((y1+y2)/2)) ($1)"; } || { echo "NOT FOUND: $1"; return 1; }; }
}
"$@"
EOF
chmod +x /tmp/ui.sh
```

**Addressing elements:** the old XML-view screens (MainActivity, ComposeActivity,
AccountActivity, Preferences…) expose `resource-id`s like
`site.warpnet.warpdroid:id/composeButton`. The Compose screens (chats, timeline post
rows) expose **no ids** — address them by visible `text`, `content-desc`, `class`, or by
screenshot-derived coordinates. Always `shot` after an action and read the PNG to confirm.

## Step 5 — pairing (the exact sequence)

```bash
ADB=~/Android/Sdk/platform-tools/adb
# 1) camera permission dialog -> deny -> app shows manual "Enter pairing data"
/tmp/ui.sh tap_attr 'text="DON’T ALLOW"' || /tmp/ui.sh tap_attr 'permission_deny_button'
# 2) focus the EditText and paste JSON IN CHUNKS (input text truncates long strings ~290 chars)
/tmp/ui.sh tap_attr 'class="android.widget.EditText"'
$ADB shell input keycombination 113 29; $ADB shell input keyevent 67   # select-all + clear
JSON=$(cat /tmp/authnodeinfo.json); i=0
while [ $i -lt ${#JSON} ]; do $ADB shell "input text '${JSON:$i:80}'"; i=$((i+80)); sleep 0.4; done
# 3) confirm
/tmp/ui.sh tap_attr 'text="Pair"'      # -> "Connect to my node" confirm screen
/tmp/ui.sh tap_attr 'text="Connect"'   # -> handshake + libp2p dial
# 4) post-pair system dialogs
/tmp/ui.sh tap_attr 'text="ALLOW"'     # "always run in background"
/tmp/ui.sh tap_attr 'permission_allow_button'   # POST_NOTIFICATIONS, if shown
```

Confirm pairing succeeded in logcat — the embedded node connects out:

```bash
$ADB logcat -d | grep -i "GoLog.*Connected out"
# e.g.  GoLog: libp2p Connected out peer=12D3KooW... remote=/ip4/.../tcp/4011
```

After this you land on MainActivity/Home with the timeline populated. Note: the `input`
uses single-quotes around the JSON — the payload has no single-quote chars, so both host
and device shells pass it through verbatim (special chars `"+/=,:[]{}` survive).

## Step 6 — screen map & selectors (verified working)

| Flow | How to reach / drive |
|------|----------------------|
| **Compose a post** | FAB `id/composeButton` → `id/composeEditField` (text) → `id/composeTweetButton` (label `TWEET!`) |
| Compose extras | `id/composeToggleVisibilityButton` (Public/Unlisted/Followers-Only/Direct), `id/composeContentWarningButton`, `id/composeAddMediaButton` (→ Take photo / Add media → system picker), `id/atButton`, `id/hashButton` |
| **Post actions** (Compose row) | reply / retweet / like / bookmark / overflow are Compose icons — tap by coords from the post's action row (`shot` first). Retweet opens a Retweet/Quote sheet then a visibility confirm |
| **Thread view** | tap a post body → `ViewThreadActivity` |
| **Account** | tap author name/avatar → `AccountActivity`; overflow (top-right) = Mute/Block/Report/Lists/… |
| **Nav drawer** | tap avatar top-left, then `text="…"`: Profile, Notifications, Direct messages, Bookmarks, Likes, Account preferences, Preferences, About, Log out |
| **Direct messages** | drawer → `text="Direct messages"` → tap a chat → `EditText` (tap upper part) + `text="Send"`; new chat = `content-desc="New message"` FAB → `text="Search people"` |
| **Search** | Home toolbar search icon → `SearchActivity` (tabs Posts/Accounts) |
| **Edit profile** | own profile → `text="Edit"` → `EditProfileActivity` |
| **Theme** | Preferences → `text="App theme"` → Dark/Light/… |

Verify each action two ways: the on-screen counter/state changes in a `shot`, and (for
mutations) logcat shows no error. Example — a like flips the star count `★ 0 → ★ 1`; a
retweet flips `⇄ 0 → ⇄ 1`; a bookmark turns the icon solid.

## Gotchas

- **`adb shell input text` truncates** long strings (~290 chars) — send in ≤80-char
  chunks with short sleeps. No spaces/newlines needed in the pairing JSON (it's compact).
- **Bottom text fields collide with the gesture nav.** Tapping the exact centre of the
  chat message `EditText` can trigger Home and background the app. Tap the **upper part**
  of its bounds. The soft keyboard also shifts the layout — re-`dump` to relocate `Send`.
- **Back can exit the app.** Some screens (Search is single-task) pop straight to Home,
  and one more Back exits to the launcher. Check `dumpsys activity activities | grep
  topResumedActivity` and relaunch with `am start` if needed rather than assuming.
- **Compose screens have no ids/testTags** — never assume a `resource-id` for chats or
  timeline rows; drive them by text/content-desc/coords.
- **The node attaches during pairing.** If the timeline is empty, check logcat for
  `Connected out`; if it never connects, the node behind `addresses` is down/unreachable
  from the emulator, or the payload is stale.
- **`netlinkrib: permission denied`** in `GoLog` is a benign emulator sandbox limitation
  (interface enumeration), not a failure — actions still succeed.

## Teardown

The emulator can stay up for reuse (`adb devices` to confirm). Clean the repo:

```bash
git checkout -- go.mod go.sum
rm -f warpdroid/node/warpnet_multi.aar warpdroid/node/warpnet_multi-sources.jar
git status --porcelain   # must be clean (no aar/go.mod/go.sum changes committed)
```

## When this skill does NOT apply

- **Verifying a node handler / `/ws` route** (no device needed) → `warpnet-testnet-verify`.
- **Adding a new protocol route/DTO across layers** → `warpnet-add-handler`.
- **A known cross-layer wire bug with a symptom** (blank DTO, DTO mismatch) →
  `warpnet-debug-stack`.
- **Pure Kotlin unit logic** with no on-screen surface → a plain build/unit run is enough.
