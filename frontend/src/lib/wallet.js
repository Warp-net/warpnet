// SPDX-License-Identifier: AGPL-3.0-or-later

// The wallet ships on testnet only, and even there it stays out of sight
// until the owner turns it on in Settings: not everyone wants one.
export function walletOn(owner) {
  return !!owner && owner.network === "testnet" && owner.wallet === true;
}
