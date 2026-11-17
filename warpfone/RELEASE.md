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
  bundle id.
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

The node encrypts its peer-to-peer traffic with standard algorithms only
(Noise with X25519, ChaCha20-Poly1305 and Ed25519 signatures), so the app sets
`ITSAppUsesNonExemptEncryption` to false. With it set to true, App Store
Connect refuses every upload that lacks the `ITSEncryptionExportComplianceCode`
of approved encryption documentation.

## App Review

There is no demo node or demo account: the app only pairs with the user's own
node, so reviewers get past the pairing screen no further. Leave "Sign-in
required" off in Beta App Review Information and App Review Information, and
explain this in the notes:

> Warpnet is a peer-to-peer network with no central server and no accounts
> held by us. The iOS app is a companion for the user's own Warpnet node
> running on their computer; it pairs with that node by scanning its QR code.
> There is no hosted service we could provide a demo account for.

If the review is rejected under guideline 2.1 for a missing demo account,
answer in the Resolution Center with the same text.

The notes should also say where moderation happens (reports go to the
network's moderators, accounts can be blocked from their profiles), since
guideline 1.2 asks apps with user-generated content to offer both.

The app bundles the AGPL-3.0 node, and Apple's standard EULA adds usage
restrictions the AGPL forbids, so App Information sets a custom license
agreement:

> Warpnet is free software. The app is licensed to you under the GNU Affero
> General Public License, version 3 or later
> (https://www.gnu.org/licenses/agpl-3.0.html). Portions derived from Ice
> Cubes are licensed under the Apache License, Version 2.0
> (https://www.apache.org/licenses/LICENSE-2.0). Source code:
> https://github.com/Warp-net/warpnet. The app is provided without any
> warranty, to the extent permitted by applicable law.
