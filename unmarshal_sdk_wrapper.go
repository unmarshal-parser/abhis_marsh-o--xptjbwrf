// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package main

import (
	"errors"
	unmarshal "github.com/eucrypt/unmarshal-go-sdk/pkg"
	sdkConstants "github.com/eucrypt/unmarshal-go-sdk/pkg/constants"
	"github.com/eucrypt/unmarshal-go-sdk/pkg/transaction_details"
	"github.com/eucrypt/unmarshal-go-sdk/pkg/transaction_details/types"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"time"
)

const MaximumPerPage = 100

var (
	Base10ChainIDMap = map[string]sdkConstants.Chain{
		"1":          sdkConstants.ETH,
		"4":          sdkConstants.ETH_RINKEBY,
		"10":         sdkConstants.OPTIMISM,
		"25":         sdkConstants.CRONOS,
		"56":         sdkConstants.BSC,
		"97":         sdkConstants.BSC_TESTNET,
		"106":        sdkConstants.VELAS,
		"122":        sdkConstants.FUSE,
		"137":        sdkConstants.MATIC,
		"250":        sdkConstants.FANTOM,
		"8217":       sdkConstants.KLAYTN,
		"42161":      sdkConstants.ARBITRUM,
		"42220":      sdkConstants.CELO,
		"43114":      sdkConstants.AVALANCHE,
		"80001":      sdkConstants.MATIC_TESTNET,
		"1284":       sdkConstants.MOONBEAM,
		"1088":       sdkConstants.METIS,
		"1313161554": sdkConstants.AURORA,
		"100":        sdkConstants.MaticSupernet,
		"1101":       sdkConstants.ZKEVM,
		"5001":       sdkConstants.MANTLE_TESTNET,
		"5000":       sdkConstants.MANTLE,
	}

	PriceSupportedChains = map[sdkConstants.Chain]bool{
		sdkConstants.ARBITRUM: true,
		sdkConstants.BSC:      true,
		sdkConstants.ETH:      true,
		sdkConstants.MATIC:    true,
	}

	WrappedTokenOnChainMap = map[sdkConstants.Chain]string{
		sdkConstants.ARBITRUM:       "0x82af49447d8a07e3bd95bd0d56f35241523fbab1",
		sdkConstants.AVALANCHE:      "0xb31f66aa3c1e785363f0875a1b74e27b85fd66c7",
		sdkConstants.BSC:            "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c",
		sdkConstants.CELO:           "0x471ece3750da237f93b8e339c536989b8978a438",
		sdkConstants.CRONOS:         "0x5c7f8a570d578ed84e63fdfa7b1ee72deae1ae23",
		sdkConstants.ETH:            "0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2",
		sdkConstants.FANTOM:         "0x21be370d5312f44cb42ce377bc9b8a0cef1a4c83",
		sdkConstants.FUSE:           "0x0be9e53fd7edac9f859882afdda116645287c629",
		sdkConstants.KLAYTN:         "0xe4f05a66ec68b54a58b17c22107b02e0232cc817",
		sdkConstants.MATIC:          "0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270",
		sdkConstants.OPTIMISM:       "0x4200000000000000000000000000000000000006",
		sdkConstants.VELAS:          "0xc579d1f3cf86749e05cd06f7ade17856c2ce3126",
		sdkConstants.MOONBEAM:       "0xacc15dc74880c9944775448304b263d191c6077f",
		sdkConstants.METIS:          "0xdeaddeaddeaddeaddeaddeaddeaddeaddead0000",
		sdkConstants.AURORA:         "0xc9bdeed33cd01541e1eed10f90519d2c06fe3feb",
		sdkConstants.ZKEVM:          "0x4f9a0e7fd2bf6067db6994cf12e4495df938e6e9",
		sdkConstants.MANTLE_TESTNET: "0xdeaddeaddeaddeaddeaddeaddeaddeaddead0000",
		sdkConstants.MANTLE:         "0xdeaddeaddeaddeaddeaddeaddeaddeaddead0000",
	}
)

func GetChainFromChainID(chainID string) (sdkConstants.Chain, error) {
	chain := Base10ChainIDMap[chainID]
	if chain == "" {
		return "", errors.New("unsupported chain error")
	}
	return chain, nil
}

func IsPriceSupportedForChain(chain sdkConstants.Chain) bool {
	return PriceSupportedChains[chain]
}

type UnmarshalSDKWrapper struct {
	unmarshalSDK    *unmarshal.Unmarshal
	chain           sdkConstants.Chain
	numberOfRetries int
	sdkMetrics      *Metrics
}

func NewUnmarshalSDKWrapper(unmarshalSDK *unmarshal.Unmarshal, chain sdkConstants.Chain, numberOfRetries int, metrics *Metrics) *UnmarshalSDKWrapper {
	return &UnmarshalSDKWrapper{
		unmarshalSDK:    unmarshalSDK,
		chain:           chain,
		numberOfRetries: numberOfRetries,
		sdkMetrics:      metrics,
	}
}

func (u UnmarshalSDKWrapper) incrementRestartCounter(location string) {
	u.sdkMetrics.UnmarshalAPIRestarts.With(
		prometheus.Labels{
			"location": location,
			"chain":    u.chain.String(),
		},
	).Inc()
}

func (u UnmarshalSDKWrapper) observeResponseLatency(start time.Time, api string) {
	u.sdkMetrics.UnmarshalAPILatency.With(prometheus.Labels{
		"chain": u.chain.String(),
		"api":   api,
	}).Observe(float64(time.Since(start)))
}

//GetAllTransactionsBetween fetches all transactions across a block range
func (u UnmarshalSDKWrapper) GetAllTransactionsBetween(from, to int, contractAddress string) ([]types.RawTransaction, error) {
	var resp = make([]types.RawTransaction, 0)
	for pageNumber := 1; true; pageNumber++ {
		fullResp, err := u.getTransactionsBetween(from, to, pageNumber, contractAddress)
		if err != nil {
			return resp, err
		}
		resp = append(resp, fullResp.Transactions...)
		if !fullResp.NextPage || len(fullResp.Transactions) == 0 {
			break
		}
	}

	return resp, nil
}

//getTransactionsBetween fetches transactions across a block range and for a page number.
func (u UnmarshalSDKWrapper) getTransactionsBetween(from, to, pageNumber int, contractAddress string) (fullResp types.RawTransactionsResponseV1, err error) {
	var (
		paginationDetails = transaction_details.TransactionDetailsOpts{
			PaginationOptions: transaction_details.PaginationOptions{
				Page:     pageNumber,
				PageSize: MaximumPerPage,
			},
			BlockLimitsOpts: transaction_details.BlockLimitsOpts{
				FromBlock: uint64(from),
				ToBlock:   uint64(to),
			},
		}
	)

	for true {
		fullResp, err = u.getTransactionsWithRetry(contractAddress, paginationDetails)
		if err != nil {
			return
		}
		if fullResp.LastVerifiedBlock == nil {
			return types.RawTransactionsResponseV1{}, errors.New("received incompatible response from gateway")
		}
		if fullResp.LastVerifiedBlock.Int64() >= int64(to) {
			return
		}
		log.WithField("Last verified block", fullResp.LastVerifiedBlock.String()).Info("Waiting for verifier to catch up.")
		time.Sleep(15 * time.Second)
	}

	return fullResp, err
}

func (u UnmarshalSDKWrapper) getTransactionsWithRetry(contractAddress string, paginationDetails transaction_details.TransactionDetailsOpts) (fullResp types.RawTransactionsResponseV1, err error) {
	defer u.observeResponseLatency(time.Now(), "GetRawTransactionsForAddress")
	for i := 0; i < u.numberOfRetries; i++ {
		fullResp, err = u.unmarshalSDK.GetRawTransactionsForAddress(u.chain, contractAddress, &paginationDetails)
		if err == nil {
			return
		}
		time.Sleep(3 * time.Second)
	}
	u.incrementRestartCounter("GetRawTransactionsForAddress")
	return
}

//GetTransactionByHash gets transaction details using the hash and assigned chain ID
func (u UnmarshalSDKWrapper) GetTransactionByHash(transactionHash string) (resp types.TxnByID, err error) {
	defer u.observeResponseLatency(time.Now(), "GetTransactionByHash")
	for i := 0; i < u.numberOfRetries; i++ {
		resp, err = u.unmarshalSDK.GetTxnDetails(u.chain, transactionHash)
		if err == nil {
			return
		}
		time.Sleep(3 * time.Second)
	}
	u.incrementRestartCounter("GetTransactionByHash")
	return
}

func (u UnmarshalSDKWrapper) GetBulkTransactionDetailsByHash(transactionHashs []string) (resp []types.TxnByID, err error) {
	defer u.observeResponseLatency(time.Now(), "GetBulkTransactionDetailsByHash")
	for i := 0; i < u.numberOfRetries; i++ {
		resp, err = u.unmarshalSDK.GetBulkTxnDetails(u.chain, transactionHashs)
		if err == nil {
			return
		}
		time.Sleep(3 * time.Second)
	}
	u.incrementRestartCounter("GetBulkTransactionDetailsByHash")
	return
}
