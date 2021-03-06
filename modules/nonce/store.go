package nonce

import (
	"fmt"

	"github.com/tendermint/go-wire"

	"github.com/cosmos/cosmos-sdk/errors"
	"github.com/cosmos/cosmos-sdk/state"
)

func getSeq(store state.SimpleDB, key []byte) (seq uint64, err error) {
	data := store.Get(key)
	if len(data) == 0 {
		//if the key is not stored, its a new key with a sequence of zero!
		return 0, nil
	}
	err = wire.ReadBinaryBytes(data, &seq)
	if err != nil {
		msg := fmt.Sprintf("Error reading sequence for %X", key)
		return seq, errors.ErrInternal(msg)
	}
	return seq, nil
}

func setSeq(store state.SimpleDB, key []byte, seq uint64) error {
	//fmt.Printf("setSeq: %s\n", hex.EncodeToString(key))
	bin := wire.BinaryBytes(seq)
	store.Set(key, bin)
	return nil // real stores can return error...
}
