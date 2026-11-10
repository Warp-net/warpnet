package handler

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAStakeFirstAskedHoldsOnceThePaymentsCheckedBeforeItHaveLanded(t *testing.T) {
	repo := &stubSponsoredSettingsRepo{}
	_, err := StreamUpdateSponsoredSettingsHandler(repo)(
		marshal(t, event.UpdateSponsoredSettingsEvent{MinStakeTRX: 500}), nil,
	)
	require.NoError(t, err)
	settings := repo.saved

	gate := settings.ClaimMinStakeTRX(settings.ChangedAt.Add(domain.MinStakeChangeWindow))
	assert.Equal(t, uint64(500), gate, "the stake holds for payments made once the change settled")
	assert.Error(t, checkPayerStake(&stubSponsoredWallet{staked: "0"}, testPayer, gate), "a wallet with no stake is refused")
	taken := stubOrders{"sold/other-buyer": domain.Order{BuyerNodeId: "other-node", Payer: testPayer, Confirmed: true}}
	assert.ErrorIs(t, checkBuyerPayer(taken, testPayer, "buyer-node", gate), ErrPayerTaken, "a payer bound to another node is refused")

	assert.Zero(t, settings.ClaimMinStakeTRX(settings.ChangedAt.Add(-time.Hour)), "a payment made before the stake was asked owes none")
}
