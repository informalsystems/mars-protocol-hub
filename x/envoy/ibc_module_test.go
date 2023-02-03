package envoy_test

import (
	"encoding/json"
	"testing"

	"github.com/CosmWasm/wasmd/x/wasm"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/libs/log"

	icatypes "github.com/cosmos/ibc-go/v6/modules/apps/27-interchain-accounts/types"

	channeltypes "github.com/cosmos/ibc-go/v6/modules/core/04-channel/types"
	marsapp "github.com/mars-protocol/hub/app"
	marsgov "github.com/mars-protocol/hub/x/gov/types"

	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	transfertypes "github.com/cosmos/ibc-go/v6/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v6/modules/core/02-client/types"
	ibctesting "github.com/cosmos/ibc-go/v6/testing"
	envoytypes "github.com/mars-protocol/hub/x/envoy/types"
	dbm "github.com/tendermint/tm-db"
)

var (
	_ ibctesting.TestingApp = (*marsapp.MarsApp)(nil)

	// TestOwnerAddress defines a reusable bech32 address for testing purposes
	TestOwnerAddress = "cosmos17dtl0mjt3t77kpuhg2edqzjpszulwhgzuj9ljs"

	// TestPortID defines a reusable port identifier for testing purposes
	TestPortID, _ = icatypes.NewControllerPortID(TestOwnerAddress)

	// TestVersion defines a reusable interchainaccounts version string for testing purposes
	TestVersion = string(icatypes.ModuleCdc.MustMarshalJSON(&icatypes.Metadata{
		Version:                icatypes.Version,
		ControllerConnectionId: ibctesting.FirstConnectionID,
		HostConnectionId:       ibctesting.FirstConnectionID,
		Encoding:               icatypes.EncodingProtobuf,
		TxType:                 icatypes.TxTypeSDKMultiMsg,
	}))
)

func SetupMarsTestingApp() (ibctesting.TestingApp, map[string]json.RawMessage) {
	encCdc := marsapp.MakeEncodingConfig()

	app := marsapp.NewMarsApp(
		log.NewNopLogger(),
		dbm.NewMemDB(),
		nil,
		true,
		map[int64]bool{},
		marsapp.DefaultNodeHome,
		5,
		encCdc,
		simapp.EmptyAppOptions{},
		[]wasm.Option{},
	)

	return (ibctesting.TestingApp)(app), marsapp.DefaultGenesisState(encCdc.Codec)
}

func init() {
	ibctesting.DefaultTestingAppInit = SetupMarsTestingApp
}

func GetMarsApp(chain *ibctesting.TestChain) *marsapp.MarsApp {
	app, ok := chain.App.(*marsapp.MarsApp)
	require.True(chain.T, ok)

	return app
}

type EnvoyTestSuite struct {
	suite.Suite

	coordinator *ibctesting.Coordinator

	// testing chains used for convenience and readability
	chainA *ibctesting.TestChain
	chainB *ibctesting.TestChain
	chainC *ibctesting.TestChain
}

func (suite *EnvoyTestSuite) SetupTest() {
	suite.coordinator = ibctesting.NewCoordinator(suite.T(), 3)
	suite.chainA = suite.coordinator.GetChain(ibctesting.GetChainID(1))
	suite.chainB = suite.coordinator.GetChain(ibctesting.GetChainID(2))
	suite.chainC = suite.coordinator.GetChain(ibctesting.GetChainID(3))
}

func NewTransferPath(chainA, chainB *ibctesting.TestChain) *ibctesting.Path {
	path := ibctesting.NewPath(chainA, chainB)
	path.EndpointA.ChannelConfig.PortID = ibctesting.TransferPort
	path.EndpointB.ChannelConfig.PortID = ibctesting.TransferPort
	path.EndpointA.ChannelConfig.Version = transfertypes.Version
	path.EndpointB.ChannelConfig.Version = transfertypes.Version

	return path
}

func NewICAPath(chainA, chainB *ibctesting.TestChain) *ibctesting.Path {
	path := ibctesting.NewPath(chainA, chainB)
	path.EndpointA.ChannelConfig.PortID = icatypes.HostPortID
	path.EndpointB.ChannelConfig.PortID = icatypes.HostPortID
	path.EndpointA.ChannelConfig.Order = channeltypes.ORDERED
	path.EndpointB.ChannelConfig.Order = channeltypes.ORDERED
	path.EndpointA.ChannelConfig.Version = TestVersion
	path.EndpointB.ChannelConfig.Version = TestVersion

	return path
}

// SetupICAPath invokes the InterchainAccounts entrypoint and subsequent channel handshake handlers
func SetupICAPath(path *ibctesting.Path, owner string) error {
	if err := RegisterInterchainAccount(path.EndpointA, owner); err != nil {
		return err
	}

	if err := path.EndpointB.ChanOpenTry(); err != nil {
		return err
	}

	if err := path.EndpointA.ChanOpenAck(); err != nil {
		return err
	}

	if err := path.EndpointB.ChanOpenConfirm(); err != nil {
		return err
	}

	return nil
}

// RegisterInterchainAccount is a helper function for starting the channel handshake
func RegisterInterchainAccount(endpoint *ibctesting.Endpoint, owner string) error {
	portID, err := icatypes.NewControllerPortID(owner)
	if err != nil {
		return err
	}

	channelSequence := endpoint.Chain.App.GetIBCKeeper().ChannelKeeper.GetNextChannelSequence(endpoint.Chain.GetContext())

	if err := GetMarsApp(endpoint.Chain).ICAControllerKeeper.RegisterInterchainAccount(endpoint.Chain.GetContext(), endpoint.ConnectionID, owner, TestVersion); err != nil {
		return err
	}

	// commit state changes for proof verification
	endpoint.Chain.NextBlock()

	// update port/channel ids
	endpoint.ChannelID = channeltypes.FormatChannelIdentifier(channelSequence)
	endpoint.ChannelConfig.PortID = portID

	return nil
}

func (suite *EnvoyTestSuite) TestHandleGov() {
	_, ok := suite.chainA.App.(*marsapp.MarsApp)
	if !ok {
		panic("not mars app")
	}

	// setup ICA path between chainA and chainB
	icaPath := NewICAPath(suite.chainA, suite.chainB)
	suite.coordinator.SetupConnections(icaPath)

	// setup ibc-transfer path between chainA and chainB
	path := NewTransferPath(suite.chainA, suite.chainB)
	suite.coordinator.Setup(path)

	envoyModuleAddress := GetMarsApp(suite.chainA).EnvoyKeeper.GetModuleAddress().String()

	// setup ICA for envoy module account
	err := SetupICAPath(icaPath, envoyModuleAddress)
	suite.Require().NoError(err) // message committed

	// registerICAMsg := &envoytypes.MsgRegisterAccount{
	// 	Sender:       suite.chainA.SenderAccount.GetAddress().String(),
	// 	ConnectionId: icaPath.EndpointA.ConnectionID,
	// }

	// packet, err := ibctesting.ParsePacketFromEvents(res.GetEvents())
	// suite.Require().NoError(err)

	icaAddrs := GetMarsApp(suite.chainA).ICAControllerKeeper.GetAllInterchainAccounts(suite.chainA.GetContext())

	suite.T().Log(icaAddrs)
	suite.T().Log(envoyModuleAddress)
	suite.T().Log(suite.chainA.SenderAccount.GetAddress().String())
	suite.T().Log(suite.chainA.SenderPrivKey)
	suite.Require().NotEmpty(icaAddrs)

	metadata := &marsgov.ProposalMetadata{
		Title:   "This is the title",
		Authors: []string{"John Doe"},
		Details: "These are the details",
	}
	metadataJsonBytes, err := json.Marshal(metadata)

	amount, ok := sdk.NewIntFromString("9223372036854775808") // 2^63 (one above int64)
	suite.Require().True(ok)
	coin := sdk.NewCoin(sdk.DefaultBondDenom, amount)

	sendFundsMsg := &envoytypes.MsgSendFunds{
		Authority: envoyModuleAddress,
		ChannelId: icaPath.EndpointA.ConnectionID,
		Amount:    sdk.NewCoins(coin),
	}

	msg, err := govv1.NewMsgSubmitProposal([]sdk.Msg{sendFundsMsg}, sdk.NewCoins(coin), suite.chainA.SenderAccount.GetAddress().String(), string(metadataJsonBytes))
	suite.Require().NoError(err) // message committed
	suite.T().Log(msg)

	res, err := suite.chainA.SendMsgs(msg)
	suite.T().Log(res)
	suite.Require().NoError(err) // message committed

	// voteMsg := govv1.NewMsgVote(testAuthority, 1, govv1.OptionYes, "")
}

// constructs a send from chainA to chainB on the established channel/connection
// and sends the same coin back from chainB to chainA.
func (suite *EnvoyTestSuite) TestHandleMsgTransfer() {
	// setup between chainA and chainB
	path := NewTransferPath(suite.chainA, suite.chainB)
	suite.coordinator.Setup(path)

	//	originalBalance := GetMarsApp(suite.chainA).BankKeeper.GetBalance(suite.chainA.GetContext(), suite.chainA.SenderAccount.GetAddress(), sdk.DefaultBondDenom)
	timeoutHeight := clienttypes.NewHeight(1, 110)

	amount, ok := sdk.NewIntFromString("9223372036854775808") // 2^63 (one above int64)
	suite.Require().True(ok)
	coinToSendToB := sdk.NewCoin(sdk.DefaultBondDenom, amount)

	// send from chainA to chainB
	msg := transfertypes.NewMsgTransfer(path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, coinToSendToB, suite.chainA.SenderAccount.GetAddress().String(), suite.chainB.SenderAccount.GetAddress().String(), timeoutHeight, 0, "")
	res, err := suite.chainA.SendMsgs(msg)

	suite.Require().NoError(err) // message committed

	packet, err := ibctesting.ParsePacketFromEvents(res.GetEvents())
	suite.Require().NoError(err)

	// relay send
	err = path.RelayPacket(packet)
	suite.Require().NoError(err) // relay committed

	// check that voucher exists on chain B
	voucherDenomTrace := transfertypes.ParseDenomTrace(transfertypes.GetPrefixedDenom(packet.GetDestPort(), packet.GetDestChannel(), sdk.DefaultBondDenom))
	balance := GetMarsApp(suite.chainB).BankKeeper.GetBalance(suite.chainB.GetContext(), suite.chainB.SenderAccount.GetAddress(), voucherDenomTrace.IBCDenom())

	coinSentFromAToB := transfertypes.GetTransferCoin(path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, sdk.DefaultBondDenom, amount)
	suite.Require().Equal(coinSentFromAToB, balance)

	// setup between chainB to chainC
	// NOTE:
	// pathBtoC.EndpointA = endpoint on chainB
	// pathBtoC.EndpointB = endpoint on chainC
	pathBtoC := NewTransferPath(suite.chainB, suite.chainC)
	suite.coordinator.Setup(pathBtoC)

	// send from chainB to chainC
	msg = transfertypes.NewMsgTransfer(pathBtoC.EndpointA.ChannelConfig.PortID, pathBtoC.EndpointA.ChannelID, coinSentFromAToB, suite.chainB.SenderAccount.GetAddress().String(), suite.chainC.SenderAccount.GetAddress().String(), timeoutHeight, 0, "")
	res, err = suite.chainB.SendMsgs(msg)
	suite.Require().NoError(err) // message committed

	packet, err = ibctesting.ParsePacketFromEvents(res.GetEvents())
	suite.Require().NoError(err)

	err = pathBtoC.RelayPacket(packet)
	suite.Require().NoError(err) // relay committed

	// NOTE: fungible token is prefixed with the full trace in order to verify the packet commitment
	fullDenomPath := transfertypes.GetPrefixedDenom(pathBtoC.EndpointB.ChannelConfig.PortID, pathBtoC.EndpointB.ChannelID, voucherDenomTrace.GetFullDenomPath())

	coinSentFromBToC := sdk.NewCoin(transfertypes.ParseDenomTrace(fullDenomPath).IBCDenom(), amount)
	balance = GetMarsApp(suite.chainC).BankKeeper.GetBalance(suite.chainC.GetContext(), suite.chainC.SenderAccount.GetAddress(), coinSentFromBToC.Denom)

	// check that the balance is updated on chainC
	suite.Require().Equal(coinSentFromBToC, balance)

	// check that balance on chain B is empty
	balance = GetMarsApp(suite.chainB).BankKeeper.GetBalance(suite.chainB.GetContext(), suite.chainB.SenderAccount.GetAddress(), coinSentFromBToC.Denom)
	suite.Require().Zero(balance.Amount.Int64())

	// send from chainC back to chainB
	msg = transfertypes.NewMsgTransfer(pathBtoC.EndpointB.ChannelConfig.PortID, pathBtoC.EndpointB.ChannelID, coinSentFromBToC, suite.chainC.SenderAccount.GetAddress().String(), suite.chainB.SenderAccount.GetAddress().String(), timeoutHeight, 0, "")
	res, err = suite.chainC.SendMsgs(msg)
	suite.Require().NoError(err) // message committed

	packet, err = ibctesting.ParsePacketFromEvents(res.GetEvents())
	suite.Require().NoError(err)

	err = pathBtoC.RelayPacket(packet)
	suite.Require().NoError(err) // relay committed

	balance = GetMarsApp(suite.chainB).BankKeeper.GetBalance(suite.chainB.GetContext(), suite.chainB.SenderAccount.GetAddress(), coinSentFromAToB.Denom)

	// check that the balance on chainA returned back to the original state
	suite.Require().Equal(coinSentFromAToB, balance)

	// check that module account escrow address is empty
	escrowAddress := transfertypes.GetEscrowAddress(packet.GetDestPort(), packet.GetDestChannel())
	balance = GetMarsApp(suite.chainB).BankKeeper.GetBalance(suite.chainB.GetContext(), escrowAddress, sdk.DefaultBondDenom)
	suite.Require().Equal(sdk.NewCoin(sdk.DefaultBondDenom, sdk.ZeroInt()), balance)

	// check that balance on chain B is empty
	balance = GetMarsApp(suite.chainC).BankKeeper.GetBalance(suite.chainC.GetContext(), suite.chainC.SenderAccount.GetAddress(), voucherDenomTrace.IBCDenom())
	suite.Require().Zero(balance.Amount.Int64())
}

func TestTransferTestSuite(t *testing.T) {
	suite.Run(t, new(EnvoyTestSuite))
}
