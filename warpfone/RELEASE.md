# Releasing warpfone

## TestFlight

The `Release / Warpfone` workflow builds an existing tag: it builds
`Warpnet.xcframework`, runs the package tests, archives the app and uploads it
to App Store Connect, from where the build goes to TestFlight. It does not
change the version; tag a commit whose `Version.xcconfig` carries the version
to ship. Each upload needs a build number App Store Connect has not seen, and
`CURRENT_PROJECT_VERSION` follows the version file, so every version can be
uploaded once. It is not part of the `Release` workflow: most releases don't
touch warpfone, so run it by hand with the tag to ship to TestFlight.

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

Reviewers pair the app with a demo node: a remote node (`Dockerfile.remote`)
on testnet, signed in to a demo account that follows a few accounts and has
posts and a chat, so the timeline, notifications and messages are not empty.

The remote node takes one dashboard login per process: a second browser gets
"already authenticated", and signing out closes the database, which cuts off
every paired device. So reviewers get pairing data, never the dashboard login:

1. Sign in to the demo node's dashboard once and stay signed in until the
   review ends.
2. Open the menu → "Sign in by QR-code", take a screenshot of the QR code and
   press "Copy connection data".
3. In Beta App Review Information and App Review Information, leave "Sign-in
   required" off, attach the screenshot and put the connection data into the
   notes below.

The pairing token lives in the node's memory: a restart replaces it and signs
the node out. Do not restart or sign out of the demo node while a build is in
review. If it restarts, sign in again and send the new QR code and connection
data in the Resolution Center. Several devices can pair with the same data.

Notes:

> Warpnet is a peer-to-peer network with no central server and no accounts
> held by us. The iOS app is a companion for the user's own Warpnet node
> running on their computer. For the review we run a demo node. To pair with
> it:
>
> 1. Point the camera at the attached QR code, or tap "Enter pairing data
>    manually", paste the connection data below and tap "Pair".
> 2. Read the content rules, turn on "I agree" and tap "Connect".
>
> Connection data: <connection data>
>
> Moderation: posts and profiles can be reported, and reports go to the
> network's moderators. Accounts can be blocked from their profiles. Before
> pairing, users agree to terms with zero tolerance for objectionable content
> and abusive users.

If the review is rejected under guideline 2.1 because pairing fails, check
that the demo node is up and signed in, and answer in the Resolution Center
with fresh connection data.

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
