package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
)

var (
	_ sdk.Msg = &MsgRegisterAccount{}
	_ sdk.Msg = &MsgSendFunds{}
	_ sdk.Msg = &MsgSendMessages{}

	// IMPORTANT: must implement this interface so that the GetCachedValue
	// method will work.
	//
	// docs:
	// https://docs.cosmos.network/main/core/encoding#interface-encoding-and-usage-of-any
	//
	// example in gov v1:
	// https://github.com/cosmos/cosmos-sdk/blob/v0.46.7/x/gov/types/v1/msgs.go#L97
	_ codectypes.UnpackInterfacesMessage = MsgSendMessages{}
)

//------------------------------------------------------------------------------
// MsgRegisterAccount
//------------------------------------------------------------------------------

// ValidateBasic does a sanity check on the provided data
func (m *MsgRegisterAccount) ValidateBasic() error {
	// the authority address must be valid
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.Wrap(ErrInvalidProposalAuthority, err.Error())
	}

	return nil
}

// GetSigners returns the expected signers for the message
func (m *MsgRegisterAccount) GetSigners() []sdk.AccAddress {
	// we have already asserted that the authority address is valid in
	// ValidateBasic, so can ignore the error here
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

//------------------------------------------------------------------------------
// MsgSendFunds
//------------------------------------------------------------------------------

// ValidateBasic does a sanity check on the provided data
func (m *MsgSendFunds) ValidateBasic() error {
	// the authority address must be valid
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.Wrap(ErrInvalidProposalAuthority, err.Error())
	}

	// the coins amount must be valid
	if err := m.Amount.Validate(); err != nil {
		return sdkerrors.Wrap(ErrInvalidProposalAmount, err.Error())
	}

	return nil
}

// GetSigners returns the expected signers for the message
func (m *MsgSendFunds) GetSigners() []sdk.AccAddress {
	// we have already asserted that the authority address is valid in
	// ValidateBasic, so can ignore the error here
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

//------------------------------------------------------------------------------
// MsgSendMessages
//------------------------------------------------------------------------------

// ValidateBasic does a sanity check on the provided data
func (m *MsgSendMessages) ValidateBasic() error {
	// the authority address must be valid
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.Wrap(ErrInvalidProposalAuthority, err.Error())
	}

	// the messages must each implement the sdk.Msg interface
	msgs, err := sdktx.GetMsgs(m.Messages, sdk.MsgTypeURL(m))
	if err != nil {
		return sdkerrors.Wrap(ErrInvalidProposalMsg, err.Error())
	}

	// there must be at least one message
	if len(msgs) < 1 {
		return sdkerrors.Wrap(ErrInvalidProposalMsg, "proposal must contain at least one message")
	}

	// all messages must be valid and have at most 1 signer
	//
	// ideally we also want to make sure that this one signer matches the
	// interchain account's address. however this is a stateful check, while in
	// ValidateBasic we're only allowed to do stateless checks.
	for _, msg := range msgs {
		if err = msg.ValidateBasic(); err != nil {
			return sdkerrors.Wrap(ErrInvalidProposalMsg, err.Error())
		}

		if len(msg.GetSigners()) > 1 {
			return sdkerrors.Wrapf(ErrInvalidProposalMsg, "msg type %s has more than 1 signers", sdk.MsgTypeURL(msg))
		}
	}

	return nil
}

// GetSigners returns the expected signers for the message
func (m *MsgSendMessages) GetSigners() []sdk.AccAddress {
	// we have already asserted that the authority address is valid in
	// ValidateBasic, so can ignore the error here
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

func (m MsgSendMessages) UnpackInterfaces(unpacker codectypes.AnyUnpacker) error {
	return sdktx.UnpackInterfaces(unpacker, m.Messages)
}
