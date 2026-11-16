# Releasing warpfone

## TestFlight

The `Release / Warpfone` workflow builds an existing tag: it builds
`Warpnet.xcframework`, runs the package tests, archives the app and uploads it
to App Store Connect, from where the build goes to TestFlight. It does not
change the version; tag a commit whose `Version.xcconfig` carries the version
to ship. Each upload needs a build number App Store Connect has not seen, and
`CURRENT_PROJECT_VERSION` follows the version file, so every version can be
uploaded once. The `Release` workflow calls it for every release, so the iOS
app ships with the desktop and Android builds; it can also be run alone.

The workflow reads these repository secrets:

- `APPLE_TEAM_ID`: the developer team that owns the `site.warpnet.warpfone`
  bundle ids and the `group.site.warpnet.warpfone` app group.
- `ASC_KEY_ID`, `ASC_ISSUER_ID`, `ASC_KEY_P8`: an App Store Connect API key
  with the App Manager role, and the contents of its `.p8` file. It fetches
  or creates the App Store profile and uploads the build.
- `DIST_CERT_P12`, `DIST_CERT_PASSWORD`: the Apple Distribution certificate
  with its private key as a base64-encoded `.p12`, and its password.

The archive is signed manually: automatic signing needs a development
profile, which Apple issues only to teams with registered devices.

### Signing setup

Done once, and again when the certificate expires. No Mac is needed.

1. In Certificates, Identifiers & Profiles, register the App ID
   `site.warpnet.warpfone` with the Fonts capability.
2. Create the certificate signing request and the private key:

   ```bash
   openssl req -new -newkey rsa:2048 -nodes -keyout dist.key -out dist.csr -subj "/CN=Warpnet Distribution"
   ```

3. Under Certificates, add an Apple Distribution certificate from `dist.csr`
   and download `distribution.cer`. Pack it with the key:

   ```bash
   openssl x509 -inform DER -in distribution.cer -out dist.pem
   openssl pkcs12 -export -legacy -inkey dist.key -in dist.pem -out dist.p12
   ```

4. Store `base64 -w0 dist.p12` and the export password as the secrets above.
   Delete `dist.key` and `dist.p12`.

## Export compliance

The app declares `ITSAppUsesNonExemptEncryption`: the node encrypts its
peer-to-peer traffic (Noise with X25519, ChaCha20-Poly1305 and Ed25519
signatures) with its own code rather than only through Apple's networking
stack. App Store Connect therefore asks for export compliance information on
the first upload. The encryption is standard and the app is sold to the
public, which usually fits the mass-market classification (ECCN 5D992.c) with
an annual self-classification report to the US Bureau of Industry and
Security; France requires a separate declaration for distribution there.
Confirm the classification before the first external release.

## App Review

Reviewers cannot run a node, so the review needs one that stays online:

1. Run a member node on a machine that is always on, joined to the network
   the build uses, reachable through the public relays.
2. Create a demo account on it with a few tweets, a follow and a chat, so the
   timeline, profiles and messages are not empty.
3. Open the node's device pairing screen and copy the pairing data.
4. In App Store Connect, put the pairing data in the review notes with these
   steps: open the app, choose to paste connection data instead of scanning,
   paste, confirm. Pairing takes up to a minute while the app reaches the
   node through a relay.

Pairing data identifies the demo node only; revoke the reviewer's device on
the node after the review.

The review notes should also say where moderation happens (reports go to the
network's moderators, accounts can be blocked from their profiles), since
guideline 1.2 asks apps with user-generated content to offer both.
