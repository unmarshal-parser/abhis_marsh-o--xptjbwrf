// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package main

import (
	"encoding/json"
	sdkTransactionTypes "github.com/eucrypt/unmarshal-go-sdk/pkg/transaction_details/types"
	"github.com/onrik/ethrpc"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"gorm.io/datatypes"
	"math/big"
	"strconv"
	"strings"
	"time"
)

var (
	_ = decimal.Decimal{}
	_ = big.NewInt
	_ = ethrpc.Transaction{}
	_ = time.Time{}
	_ = strings.Builder{}
	_ = sdkTransactionTypes.RawTransaction{}
)

func getJSONFromInterface(data interface{}) datatypes.JSON {
	var (
		err  error
		temp datatypes.JSON
	)
	temp, err = json.Marshal(data)
	if err != nil {
		log.Error("Error Marshalling Data: " + err.Error())
	}
	return temp
}

func convertToApprovalEvent(abiEvent *ABIApprovalEvent,
	transaction sdkTransactionTypes.TxnByID, chainID string) ApprovalEvent {
	return ApprovalEvent{
		EventOwner:   strings.ToLower(abiEvent.Owner.String()),
		EventSpender: strings.ToLower(abiEvent.Spender.String()),
		EventValue:   decimal.NewFromBigInt(abiEvent.Value, 0),

		BlockHash:       strings.ToLower(abiEvent.Raw.BlockHash),
		BlockNumber:     uint64(abiEvent.Raw.BlockNumber),
		BlockTime:       time.Unix(int64(transaction.Date), 0),
		ChainID:         chainID,
		ContractAddress: strings.ToLower(abiEvent.Raw.Address),
		Gas:             getDecimalFromString(transaction.Fee),
		GasPrice:        getDecimalFromString(transaction.GasPrice),
		Index:           uint(abiEvent.Raw.LogIndex),
		TxFrom:          strings.ToLower(transaction.From),
		TxHash:          strings.ToLower(abiEvent.Raw.TransactionHash),
		TxIndex:         uint(abiEvent.Raw.TransactionIndex),
		TxTo:            strings.ToLower(transaction.To),
		TxValue:         getDecimalFromString(transaction.Value),
	}
}

func getUint64FromString(numberString string) uint64 {
	number, err := strconv.ParseUint(numberString, 10, 64)
	if err != nil {
		return 0
	}
	return number
}

func getDecimalFromString(numberString string) decimal.Decimal {
	number, err := decimal.NewFromString(numberString)
	if err != nil {
		return decimal.NewFromInt(0)
	}
	return number
}
