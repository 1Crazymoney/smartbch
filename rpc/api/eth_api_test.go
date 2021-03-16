package api

import (
	"bytes"
	"encoding/hex"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"

	modbtypes "github.com/moeing-chain/MoeingDB/types"
	"github.com/moeing-chain/MoeingEVM/types"

	"github.com/moeing-chain/moeing-chain/api"
	"github.com/moeing-chain/moeing-chain/app"
	"github.com/moeing-chain/moeing-chain/internal/ethutils"
	"github.com/moeing-chain/moeing-chain/internal/testutils"
	"github.com/moeing-chain/moeing-chain/rpc/internal/ethapi"
)

const counterContract = `
// SPDX-License-Identifier: MIT
pragma solidity >=0.6.0;

contract Counter {

int public counter;

function update(int n) public {
  counter += n;
}

}
`

// code, rtCode, _ := testutils.MustCompileSolStr(counterContract)
var counterContractCreationBytecode = testutils.HexToBytes(`
608060405234801561001057600080fd5b5060b28061001f6000396000f3fe60
80604052348015600f57600080fd5b506004361060325760003560e01c806361
bc221a1460375780636299a6ef14604f575b600080fd5b603d606b565b604080
51918252519081900360200190f35b6069600480360360208110156063576000
80fd5b50356071565b005b60005481565b60008054909101905556fea2646970
6673582212205df2a10ba72894ded3e0a7ea8c57a79906cca125c3aafe3c979f
bd57e662c01d64736f6c634300060c0033
`)
var counterContractRuntimeBytecode = testutils.HexToBytes(`
6080604052348015600f57600080fd5b506004361060325760003560e01c8063
61bc221a1460375780636299a6ef14604f575b600080fd5b603d606b565b6040
8051918252519081900360200190f35b60696004803603602081101560635760
0080fd5b50356071565b005b60005481565b60008054909101905556fea26469
706673582212205df2a10ba72894ded3e0a7ea8c57a79906cca125c3aafe3c97
9fbd57e662c01d64736f6c634300060c0033
`)
var counterContractABI = testutils.MustParseABI(`
[
  {
	"inputs": [],
	"name": "counter",
	"outputs": [
	  {
		"internalType": "int256",
		"name": "",
		"type": "int256"
	  }
	],
	"stateMutability": "view",
	"type": "function"
  },
  {
	"inputs": [
	  {
		"internalType": "int256",
		"name": "n",
		"type": "int256"
	  }
	],
	"name": "update",
	"outputs": [],
	"stateMutability": "nonpayable",
	"type": "function"
  }
]
`)

func TestAccounts(t *testing.T) {
	key1, addr1 := testutils.GenKeyAndAddr()
	key2, addr2 := testutils.GenKeyAndAddr()

	_app := app.CreateTestApp(key1, key2)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	addrs, err := _api.Accounts()
	require.NoError(t, err)
	require.Len(t, addrs, 2)
	require.Contains(t, addrs, addr1)
	require.Contains(t, addrs, addr2)
}

func TestChainId(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	id := _api.ChainId()
	require.Equal(t, "0x1", id.String())
}

func TestCoinbase(t *testing.T) {
	// TODO
}

func TestGasPrice(t *testing.T) {
	// TODO
}

func TestBlockNum(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	ctx := _app.GetContext(app.RunTxMode)
	ctx.Db.AddBlock(&modbtypes.Block{Height: 0x100}, -1)
	ctx.Db.AddBlock(nil, -1) //To Flush
	ctx.Close(true)

	num, err := _api.BlockNumber()
	require.NoError(t, err)
	require.Equal(t, "0x100", num.String())
}

func TestGetBalance(t *testing.T) {
	key, addr := testutils.GenKeyAndAddr()
	_app := app.CreateTestApp(key)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	b, err := _api.GetBalance(addr, -1)
	require.NoError(t, err)
	require.Equal(t, "0x989680", b.String())
}

func TestGetTxCount(t *testing.T) {
	key, addr := testutils.GenKeyAndAddr()
	_app := app.CreateTestApp(key)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	acc := types.ZeroAccountInfo()
	acc.UpdateNonce(78)
	ctx := _app.GetContext(app.RunTxMode)
	ctx.SetAccount(addr, acc)
	ctx.Close(true)
	_app.TxEngine.Context().Close(false)
	_app.Trunk.Close(true)

	nonce, err := _api.GetTransactionCount(addr, 0)
	require.NoError(t, err)
	require.Equal(t, hexutil.Uint64(78), *nonce)
}

func TestGetCode(t *testing.T) {
	key, addr := testutils.GenKeyAndAddr()
	_app := app.CreateTestApp(key)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	ctx := _app.GetContext(app.RunTxMode)
	code := bytes.Repeat([]byte{0xff}, 32)
	code = append(code, 0x12, 0x34)
	ctx.SetCode(addr, types.NewBytecodeInfo(code))
	ctx.Close(true)
	_app.TxEngine.Context().Close(false)
	_app.Trunk.Close(true)

	c, err := _api.GetCode(addr, 0)
	require.NoError(t, err)
	require.Equal(t, "0x1234", c.String())
}

func TestGetStorageAt(t *testing.T) {
	key, addr := testutils.GenKeyAndAddr()
	_app := app.CreateTestApp(key)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	ctx := _app.GetContext(app.RunTxMode)
	code := bytes.Repeat([]byte{0xff}, 32)
	code = append(code, 0x12, 0x34)

	seq := ctx.GetAccount(addr).Sequence()
	sKey := strings.Repeat("abcd", 8)
	ctx.SetStorageAt(seq, sKey, []byte{0x12, 0x34})
	ctx.Close(true)
	_app.TxEngine.Context().Close(false)
	_app.Trunk.Close(true)

	sVal, err := _api.GetStorageAt(addr, sKey, 0)
	require.NoError(t, err)
	require.Equal(t, "0x1234", sVal.String())
}

func TestGetBlockByHash(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	hash := gethcmn.Hash{0x12, 0x34}
	block := newMdbBlock(hash, 123, nil)
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	block2, err := _api.GetBlockByHash(hash, true)
	require.NoError(t, err)
	require.Equal(t, hexutil.Uint64(123), block2["number"])
	require.Equal(t, hexutil.Bytes(hash[:]), block2["hash"])
	require.Equal(t, hexutil.Uint64(200000000), block2["gasLimit"])
	require.Equal(t, hexutil.Uint64(0), block2["gasUsed"])
	// TODO: check more fields
}

func TestGetBlockByHash_notFound(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	blk, err := _api.GetBlockByHash(gethcmn.Hash{0x12, 0x34}, true)
	require.NoError(t, err)
	require.Nil(t, blk)
}

func TestGetBlockByNum(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	hash := gethcmn.Hash{0x12, 0x34}
	block := newMdbBlock(hash, 123, []gethcmn.Hash{
		{0x56}, {0x78}, {0x90},
	})
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	block2, err := _api.GetBlockByNumber(123, true)
	require.NoError(t, err)
	require.Equal(t, hexutil.Uint64(123), block2["number"])
	require.Equal(t, hexutil.Bytes(hash[:]), block2["hash"])
	require.Len(t, block2["transactions"], 3)
	// TODO: check more fields
}

func TestGetBlockByNum_notFound(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	blk, err := _api.GetBlockByNumber(99, true)
	require.NoError(t, err)
	require.Nil(t, blk)
}

func TestGetBlockTxCountByHash(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	hash := gethcmn.Hash{0x12, 0x34}
	block := newMdbBlock(hash, 123, []gethcmn.Hash{
		{0x56}, {0x78}, {0x90},
	})
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	cnt := _api.GetBlockTransactionCountByHash(hash)
	require.Equal(t, hexutil.Uint(3), *cnt)
}

func TestGetBlockTxCountByNum(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	hash := gethcmn.Hash{0x12, 0x34}
	block := newMdbBlock(hash, 123, []gethcmn.Hash{
		{0x56}, {0x78}, {0x90}, {0xAB},
	})
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	cnt, err := _api.GetBlockTransactionCountByNumber(123)
	require.NoError(t, err)
	require.Equal(t, hexutil.Uint(4), *cnt)
}

func TestGetTxByBlockHashAndIdx(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	blkHash := gethcmn.Hash{0x12, 0x34}
	block := newMdbBlock(blkHash, 123, []gethcmn.Hash{
		{0x56}, {0x78}, {0x90}, {0xAB},
	})
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	tx, err := _api.GetTransactionByBlockHashAndIndex(blkHash, 2)
	require.NoError(t, err)
	require.Equal(t, gethcmn.Hash{0x90}, tx.Hash)
	// TODO: check more fields
}

func TestGetTxByBlockNumAndIdx(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	blkHash := gethcmn.Hash{0x12, 0x34}
	block := newMdbBlock(blkHash, 123, []gethcmn.Hash{
		{0x56}, {0x78}, {0x90}, {0xAB},
	})
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	tx, err := _api.GetTransactionByBlockNumberAndIndex(123, 1)
	require.NoError(t, err)
	require.Equal(t, gethcmn.Hash{0x78}, tx.Hash)
	// TODO: check more fields
}

func TestGetTxByHash(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	blkHash := gethcmn.Hash{0x12, 0x34}
	block := newMdbBlock(blkHash, 123, []gethcmn.Hash{
		{0x56}, {0x78}, {0x90}, {0xAB},
	})
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	tx, err := _api.GetTransactionByHash(gethcmn.Hash{0x78})
	require.NoError(t, err)
	require.Equal(t, gethcmn.Hash{0x78}, tx.Hash)
	// TODO: check more fields
}

func TestGetTxReceipt(t *testing.T) {
	_app := app.CreateTestApp()
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	blkHash := gethcmn.Hash{0x12, 0x34}
	block := testutils.NewMdbBlockBuilder().
		Hash(blkHash).Height(123).
		Tx(gethcmn.Hash{0x56}).
		Tx(gethcmn.Hash{0x78},
			types.Log{Address: gethcmn.Address{0xA1}, Topics: [][32]byte{{0xF1}, {0xF2}}},
			types.Log{Address: gethcmn.Address{0xA2}, Topics: [][32]byte{{0xF3}, {0xF4}}, Data: []byte{0xD1}}).
		Tx(gethcmn.Hash{0x90}).
		Tx(gethcmn.Hash{0xAB}).
		Build()
	ctx := _app.GetContext(app.RunTxMode)
	ctx.StoreBlock(block)
	ctx.StoreBlock(nil) // flush previous block
	ctx.Close(true)

	receipt, err := _api.GetTransactionReceipt(gethcmn.Hash{0x78})
	require.NoError(t, err)
	require.Equal(t, gethcmn.Hash{0x78}, receipt["transactionHash"])
	require.Equal(t, hexutil.Uint(0x1), receipt["status"])
	require.Len(t, receipt["logs"], 2)

	gethLogs := receipt["logs"].([]*gethtypes.Log)
	require.Equal(t, gethcmn.Address{0xA2}, gethLogs[1].Address)
	require.Equal(t, []byte{0xD1}, gethLogs[1].Data)
	// TODO: check more fields
}

func TestSendRawTx(t *testing.T) {
	// TODO
}

func TestSendTx(t *testing.T) {
	// TODO
}

//func TestCall_NoFromAddr(t *testing.T) {
//	_app := app.CreateTestApp()
//	defer app.DestroyTestApp(_app)
//	_api := createEthAPI(_app)
//
//	_, err := _api.Call(ethapi.CallArgs{}, 0)
//	require.Error(t, err)
//	require.Equal(t, "missing from address", err.Error())
//}

func TestCall_Transfer(t *testing.T) {
	fromKey, fromAddr := testutils.GenKeyAndAddr()
	toKey, toAddr := testutils.GenKeyAndAddr()

	_app := app.CreateTestApp(fromKey, toKey)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	ret, err := _api.Call(ethapi.CallArgs{
		From:  &fromAddr,
		To:    &toAddr,
		Value: testutils.ToHexutilBig(10),
	}, 0)
	require.NoError(t, err)
	require.Equal(t, []byte{}, []byte(ret))

	ret, err = _api.Call(ethapi.CallArgs{
		From:  &fromAddr,
		To:    &toAddr,
		Value: testutils.ToHexutilBig(math.MaxInt64),
	}, 0)
	require.Error(t, err)
	//require.Equal(t, []byte{}, []byte(ret))
}

func TestCall_DeployContract(t *testing.T) {
	fromKey, fromAddr := testutils.GenKeyAndAddr()

	_app := app.CreateTestApp(fromKey)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	ret, err := _api.Call(ethapi.CallArgs{
		From: &fromAddr,
		Data: testutils.ToHexutilBytes(counterContractCreationBytecode),
	}, 0)
	require.NoError(t, err)
	require.Equal(t, []byte{}, []byte(ret))
}

func TestCall_RunGetter(t *testing.T) {
	fromKey, fromAddr := testutils.GenKeyAndAddr()

	_app := app.CreateTestApp(fromKey)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	// deploy contract
	tx := gethtypes.NewContractCreation(0, big.NewInt(0), 100000, big.NewInt(1),
		counterContractCreationBytecode)
	tx = ethutils.MustSignTx(tx, _app.ChainID().ToBig(), ethutils.MustHexToPrivKey(fromKey))
	testutils.ExecTxInBlock(_app, 1, tx)
	contractAddr := gethcrypto.CreateAddress(fromAddr, tx.Nonce())
	rtCode, err := _api.GetCode(contractAddr, 0)
	require.NoError(t, err)
	require.True(t, len(rtCode) > 0)

	// call contract
	data, err := counterContractABI.Pack("counter")
	require.NoError(t, err)
	results, err := _api.Call(ethapi.CallArgs{
		//From: &fromAddr,
		To:   &contractAddr,
		Data: testutils.ToHexutilBytes(data),
	}, 0)
	require.NoError(t, err)
	require.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000",
		hex.EncodeToString(results))
}

func TestEstimateGas(t *testing.T) {
	fromKey, fromAddr := testutils.GenKeyAndAddr()

	_app := app.CreateTestApp(fromKey)
	defer app.DestroyTestApp(_app)
	_api := createEthAPI(_app)

	ret, err := _api.EstimateGas(ethapi.CallArgs{
		From: &fromAddr,
		Data: testutils.ToHexutilBytes(counterContractCreationBytecode),
	})
	require.NoError(t, err)
	require.Equal(t, 96908, int(ret))
}

func createEthAPI(_app *app.App) *ethAPI {
	backend := api.NewBackend(nil, _app)
	return newEthAPI(backend, _app.TestKeys(), _app.Logger())
}

func newMdbBlock(hash gethcmn.Hash, height int64,
	txs []gethcmn.Hash) *modbtypes.Block {

	b := testutils.NewMdbBlockBuilder().Hash(hash).Height(height)
	for _, txHash := range txs {
		b.Tx(txHash, []types.Log{
			{BlockNumber: uint64(height)},
		}...)
	}
	return b.Build()
}
