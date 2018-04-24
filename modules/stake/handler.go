package stake

import (
	"fmt"
	"strconv"

	"github.com/CyberMiles/travis/commons"
	"github.com/CyberMiles/travis/types"
	"github.com/CyberMiles/travis/utils"
	"github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/errors"
	"github.com/cosmos/cosmos-sdk/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/tendermint/go-wire/data"
	"math/big"
)

// nolint
const (
	stakingModuleName = "stake"
	foundationAddress = "0x" // fixme move to config file
)

//_______________________________________________________________________

// DelegatedProofOfStake - interface to enforce delegation stake
type delegatedProofOfStake interface {
	declareCandidacy(TxDeclareCandidacy) error
	updateCandidacy(TxUpdateCandidacy) error
	withdrawCandidacy(TxWithdrawCandidacy) error
	verifyCandidacy(TxVerifyCandidacy) error
	delegate(TxDelegate) error
	withdraw(TxWithdraw) error
}

//_______________________________________________________________________

// InitState - set genesis parameters for staking
func InitState(key, value string, store state.SimpleDB) error {
	params := loadParams(store)
	switch key {
	case "reserve_requirement_ratio":
		params.ReserveRequirementRatio = value
	case "max_vals":
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("input must be integer, Error: %v", err.Error())
		}

		params.MaxVals = uint16(i)
	case "validator":
		setValidator(value)
	default:
		return errors.ErrUnknownKey(key)
	}

	saveParams(store, params)
	return nil
}

func setValidator(value string) error {
	var val genesisValidator
	err := data.FromJSON([]byte(value), &val)
	if err != nil {
		return fmt.Errorf("error reading validators")
	}

	if val.Address == common.HexToAddress("0000000000000000000000000000000000000000") {
		return ErrBadValidatorAddr()
	}

	// create and save the empty candidate
	bond := GetCandidateByAddress(val.Address)
	if bond != nil {
		return ErrCandidateExistsAddr()
	}

	shares := new(big.Int)
	shares.Mul(big.NewInt(val.Power), big.NewInt(1e18))
	maxShares := new(big.Int)
	maxShares.Mul(big.NewInt(val.MaxAmount), big.NewInt(1e18))

	// fixme check to see if the candidate fulfills the condition to be a validator
	candidate := NewCandidate(val.PubKey, val.Address, shares, val.Power, maxShares, val.Cut, Description{}, "N")
	SaveCandidate(candidate)
	return nil
}

// CheckTx checks if the tx is properly structured
func CheckTx(ctx types.Context, store state.SimpleDB, tx sdk.Tx) (res sdk.CheckResult, err error) {
	err = tx.ValidateBasic()
	if err != nil {
		return res, err
	}

	// get the sender
	sender, err := getTxSender(ctx)
	if err != nil {
		return res, err
	}

	params := loadParams(store)
	checker := check{
		store:    store,
		sender:   sender,
		params:   params,
		ethereum: ctx.Ethereum(),
	}

	switch txInner := tx.Unwrap().(type) {
	case TxDeclareCandidacy:
		return res, checker.declareCandidacy(txInner)
	case TxUpdateCandidacy:
		return res, checker.updateCandidacy(txInner)
	case TxWithdrawCandidacy:
		return res, checker.withdrawCandidacy(txInner)
	case TxVerifyCandidacy:
		return res, checker.verifyCandidacy(txInner)
	case TxDelegate:
		return res, checker.delegate(txInner)
	case TxWithdraw:
		return res, checker.withdraw(txInner)
	}

	return res, errors.ErrUnknownTxType(tx)
}

// DeliverTx executes the tx if valid
func DeliverTx(ctx types.Context, store state.SimpleDB, tx sdk.Tx, hash []byte) (res sdk.DeliverResult, err error) {
	_, err = CheckTx(ctx, store, tx)
	if err != nil {
		return
	}

	sender, err := getTxSender(ctx)
	if err != nil {
		return
	}

	params := loadParams(store)
	deliverer := deliver{
		store:    store,
		sender:   sender,
		params:   params,
		ethereum: ctx.Ethereum(),
	}

	// Run the transaction
	switch _tx := tx.Unwrap().(type) {
	case TxDeclareCandidacy:
		return res, deliverer.declareCandidacy(_tx)
	case TxUpdateCandidacy:
		return res, deliverer.updateCandidacy(_tx)
	case TxWithdrawCandidacy:
		return res, deliverer.withdrawCandidacy(_tx)
	case TxVerifyCandidacy:
		return res, deliverer.verifyCandidacy(_tx)
	case TxDelegate:
		return res, deliverer.delegate(_tx)
	case TxWithdraw:
		return res, deliverer.withdraw(_tx)
	}

	return
}

// get the sender from the ctx and ensure it matches the tx pubkey
func getTxSender(ctx types.Context) (sender common.Address, err error) {
	senders := ctx.GetSigners()
	if len(senders) != 1 {
		return sender, ErrMissingSignature()
	}
	return senders[0], nil
}

//_______________________________________________________________________

type check struct {
	store    state.SimpleDB
	sender   common.Address
	params   Params
	ethereum *eth.Ethereum
}

var _ delegatedProofOfStake = check{} // enforce interface at compile time

func (c check) declareCandidacy(tx TxDeclareCandidacy) error {
	// check to see if the pubkey or address has been registered before
	candidate := GetCandidateByAddress(c.sender)
	if candidate != nil {
		return fmt.Errorf("address has been declared")
	}

	candidate = GetCandidateByPubKey(tx.PubKey.KeyString())
	if candidate != nil {
		return fmt.Errorf("pubkey has been declared")
	}

	// check to see if the associated account has 10%(RRR, short for Reserve Requirement Ratio, configurable) of the max staked CMT amount
	z := new(big.Float)
	x, _ := new(big.Float).SetString(tx.MaxAmount)
	y, _ := new(big.Float).SetString(c.params.ReserveRequirementRatio)
	z.Mul(x, y)
	txDelegate := TxDelegate{ValidatorAddress: c.sender, Amount: z.String()}
	return c.delegate(txDelegate)
}

func (c check) updateCandidacy(tx TxUpdateCandidacy) error {
	candidate := GetCandidateByAddress(c.sender)
	if candidate == nil {
		return fmt.Errorf("cannot edit non-exsits candidacy")
	}

	// If the max amount of CMTs is updated, the 10% of self-staking will be re-computed,
	// and the different will be charged or refunded from / into the new account address.
	if tx.MaxAmount != "" {
		rr := new(big.Float)
		x, _ := new(big.Float).SetString(tx.MaxAmount)
		y, _ := new(big.Float).SetString(c.params.ReserveRequirementRatio)
		rr.Quo(x, y)
		balance, err := commons.GetBalance(c.ethereum, c.sender)
		if err != nil {
			return err
		}

		tmp, _ := new(big.Int).SetString(rr.String(), 10)
		if balance.Cmp(tmp) < 0 {
			return ErrInsufficientFunds()
		}
	}

	// todo check to see if the candidate is changed, if not, raise error.

	return nil
}

func (c check) withdrawCandidacy(tx TxWithdrawCandidacy) error {
	// check to see if the address has been registered before
	candidate := GetCandidateByAddress(c.sender)
	if candidate == nil {
		return fmt.Errorf("cannot withdraw non-exsits candidacy")
	}

	return nil
}

func (c check) verifyCandidacy(tx TxVerifyCandidacy) error {
	// check to see if the candidate address to be verified has been registered before
	candidate := GetCandidateByAddress(tx.CandidateAddress)
	if candidate == nil {
		return fmt.Errorf("cannot verify non-exsits candidacy")
	}

	// check to see if the request was initiated by a special account
	if c.sender != common.HexToAddress(foundationAddress) {
		return ErrVerificationDisallowed()
	}

	if candidate.Verified == "Y" {
		return ErrVerifiedAlready()
	}

	return nil
}

func (c check) delegate(tx TxDelegate) error {
	candidate := GetCandidateByAddress(tx.ValidatorAddress)
	if candidate == nil {
		return ErrNoCandidateForAddress()
	}

	// check if the delegator has sufficient funds
	balance, err := commons.GetBalance(c.ethereum, c.sender)
	if err != nil {
		return err
	}

	amount := new(big.Int)
	_, ok := amount.SetString(tx.Amount, 10)
	if !ok {
		return ErrBadAmount()
	}

	if balance.Cmp(amount) < 0 {
		return ErrInsufficientFunds()
	}

	// check to see if the validator has reached its declared max amount CMTs to be staked.
	x := new(big.Int)
	x.Add(candidate.Shares, amount)
	if x.Cmp(candidate.MaxShares) > 0 {
		return ErrReachMaxAmount()
	}

	return nil
}

func (c check) withdraw(tx TxWithdraw) error {
	// check if has delegated
	candidate := GetCandidateByAddress(tx.ValidatorAddress)
	if candidate == nil {
		return ErrBadValidatorAddr()
	}

	delegation := GetDelegation(c.sender, tx.ValidatorAddress)
	if delegation == nil {
		return ErrDelegationNotExists()
	}

	return nil
}

//_____________________________________________________________________

type deliver struct {
	store    state.SimpleDB
	sender   common.Address
	params   Params
	ethereum *eth.Ethereum
}

var _ delegatedProofOfStake = deliver{} // enforce interface at compile time

// These functions assume everything has been authenticated,
// now we just perform action and save
func (d deliver) declareCandidacy(tx TxDeclareCandidacy) error {
	// create and save the empty candidate
	maxAmount, ok := new(big.Int).SetString(tx.MaxAmount, 10)
	if !ok {
		return ErrBadAmount()
	}

	candidate := NewCandidate(tx.PubKey, d.sender, big.NewInt(0), 0, maxAmount, tx.Cut, tx.Description, "N")
	SaveCandidate(candidate)

	// delegate a part of the max staked CMT amount
	z := new(big.Float)
	x, _ := new(big.Float).SetString(tx.MaxAmount)
	y, _ := new(big.Float).SetString(d.params.ReserveRequirementRatio)
	z.Mul(x, y)
	txDelegate := TxDelegate{ValidatorAddress: d.sender, Amount: z.String()}
	return d.delegate(txDelegate)
}

func (d deliver) updateCandidacy(tx TxUpdateCandidacy) error {
	// create and save the empty candidate
	candidate := GetCandidateByAddress(d.sender)
	if candidate == nil {
		return ErrNoCandidateForAddress()
	}

	// If the max amount of CMTs is updated, the 10% of self-staking will be re-computed,
	// and the different will be charged or refunded from / into the new account address.
	maxAmount, _ := new(big.Int).SetString(tx.MaxAmount, 10)
	if candidate.MaxShares.Cmp(maxAmount) != 0 {
		tmp := new(big.Int)
		diff := tmp.Sub(candidate.MaxShares, maxAmount)
		x, _ := new(big.Float).SetString(diff.String())
		y, _ := new(big.Float).SetString(d.params.ReserveRequirementRatio)
		z := new(big.Float)
		z.Mul(x, y)

		amount, _ := new(big.Int).SetString(z.String(), 10)
		if diff.Cmp(big.NewInt(0)) > 0 {
			// charge
			commons.Transfer(d.sender, DefaultHoldAccount, amount)
		} else {
			// refund
			commons.Transfer(DefaultHoldAccount, d.sender, amount)
		}

		candidate.MaxShares = maxAmount
		shares := new(big.Int)
		shares.Add(candidate.Shares, amount)
		candidate.Shares = tmp
	}

	candidate.OwnerAddress = tx.NewAddress
	candidate.Verified = "N"
	candidate.UpdatedAt = utils.GetNow()
	updateCandidate(candidate)
	return nil
}

func (d deliver) withdrawCandidacy(tx TxWithdrawCandidacy) error {
	// create and save the empty candidate
	validatorAddress := d.sender
	candidate := GetCandidateByAddress(validatorAddress)
	if candidate == nil {
		return ErrNoCandidateForAddress()
	}

	// All staked tokens will be distributed back to delegator addresses.
	// Self-staked CMTs will be refunded back to the validator address.
	delegations := GetDelegationsByCandidate(d.sender)
	for _, delegation := range delegations {
		err := commons.Transfer(d.params.HoldAccount, delegation.DelegatorAddress, delegation.Shares)
		if err != nil {
			return err
		}
		RemoveDelegation(delegation)
	}

	removeCandidate(candidate)
	return nil
}

func (d deliver) verifyCandidacy(tx TxVerifyCandidacy) error {
	// verify candidacy
	candidate := GetCandidateByAddress(tx.CandidateAddress)
	if tx.Verified {
		candidate.Verified = "Y"
	} else {
		candidate.Verified = "N"
	}
	candidate.UpdatedAt = utils.GetNow()
	updateCandidate(candidate)
	return nil
}

func (d deliver) delegate(tx TxDelegate) error {
	// Get the pubKey bond account
	candidate := GetCandidateByAddress(tx.ValidatorAddress)

	shares, ok := new(big.Int).SetString(tx.Amount, 10)
	if !ok {
		return ErrBadAmount()
	}

	// Move coins from the delegator account to the pubKey lock account
	err := commons.Transfer(d.sender, d.params.HoldAccount, shares)
	if err != nil {
		return err
	}

	// create or update delegation
	now := utils.GetNow()
	delegation := GetDelegation(d.sender, tx.ValidatorAddress)
	if delegation == nil {
		delegation = &Delegation{
			DelegatorAddress: d.sender,
			CandidateAddress: tx.ValidatorAddress,
			Shares:           shares,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		SaveDelegation(delegation)
	} else {
		delegation.Shares.Add(delegation.Shares, shares)
		delegation.UpdatedAt = now
		UpdateDelegation(delegation)
	}

	// Add shares to candidate
	candidate.Shares.Add(candidate.Shares, shares)
	delegateHistory := &DelegateHistory{0, d.sender, tx.ValidatorAddress, shares, "delegate", now}
	updateCandidate(candidate)
	saveDelegateHistory(delegateHistory)
	return nil
}

func (d deliver) withdraw(tx TxWithdraw) error {
	// get pubKey candidate
	candidate := GetCandidateByAddress(tx.ValidatorAddress)
	if candidate == nil {
		return ErrNoCandidateForAddress()
	}

	delegation := GetDelegation(d.sender, tx.ValidatorAddress)
	RemoveDelegation(delegation)

	// deduct shares from the candidate
	candidate.Shares.Sub(candidate.Shares, delegation.Shares)
	if candidate.Shares.Cmp(big.NewInt(0)) == 0 {
		//candidate.State = "N"
		removeCandidate(candidate)
	}

	now := utils.GetNow()
	candidate.UpdatedAt = now
	updateCandidate(candidate)

	delegateHistory := &DelegateHistory{0, d.sender, tx.ValidatorAddress, big.NewInt(0), "withdraw", now}
	saveDelegateHistory(delegateHistory)

	// transfer coins back to account
	return commons.Transfer(d.params.HoldAccount, d.sender, delegation.Shares)
}
