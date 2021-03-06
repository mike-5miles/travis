package commands

import (
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/CyberMiles/travis/client/commands"
	txcmd "github.com/CyberMiles/travis/client/commands/txs"
	"github.com/CyberMiles/travis/modules/nonce"
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/ethereum/go-ethereum/common"
)

// nolint
const (
	FlagSequence = "sequence"
	FlagNonceKey = "nonce-key"
)

// NonceWrapper wraps a tx with a nonce
type NonceWrapper struct{}

var _ txcmd.Wrapper = NonceWrapper{}

// Wrap grabs the sequence number from the flag and wraps
// the tx with this nonce.  Grabs the permission from the signer,
// as we still only support single sig on the cli
func (NonceWrapper) Wrap(tx sdk.Tx) (res sdk.Tx, err error) {

	signers, err := readNonceKey()
	if err != nil {
		return res, err
	}

	seq, err := readSequence(signers)
	if err != nil {
		return res, err
	}

	res = nonce.NewTx(seq, signers, tx)
	return
}

// Register adds the sequence flags to the cli
func (NonceWrapper) Register(fs *pflag.FlagSet) {
	fs.Int(FlagSequence, -1, "Sequence number for this transaction")
	fs.String(FlagNonceKey, "", "Set of comma-separated addresses for the nonce (for multisig)")
}

func readNonceKey() ([]common.Address, error) {
	nonce := viper.GetString(FlagNonceKey)
	if nonce == "" {
		return []common.Address{txcmd.GetSigner()}, nil
	}
	return commands.ParseActors(nonce)
}

// read the sequence from the flag or query for it if flag is -1
func readSequence(signers []common.Address) (seq uint64, err error) {
	//add the nonce tx layer to the tx
	seqFlag := viper.GetInt(FlagSequence)

	switch {
	case seqFlag > 0:
		seq = uint64(seqFlag)

	case seqFlag == -1:
		//auto calculation for default sequence
		seq, _, err = doNonceQuery(signers)
		if err != nil {
			return
		}
		//fmt.Printf("doNonceQuery: %d\n", seq)
		//increase the sequence by 1!
		seq++

	default:
		err = fmt.Errorf("sequence must be either greater than 0, or -1 for autocalculation")
	}

	return
}
