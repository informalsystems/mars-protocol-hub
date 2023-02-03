package envoy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	marsapptesting "github.com/mars-protocol/hub/app/testing"

	envoytypes "github.com/mars-protocol/hub/x/envoy/types"
)

// TestCreatesModuleAccountAtGenesis asserts that the module account is properly
// registered at the auth module during genesis.
func TestCreatesModuleAccountAtGenesis(t *testing.T) {
	app := marsapptesting.MakeSimpleMockApp()
	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	acc := app.AccountKeeper.GetAccount(ctx, authtypes.NewModuleAddress(envoytypes.ModuleName))
	require.NotNil(t, acc)
}
